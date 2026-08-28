package clientapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/transport"
	"haovpn/internal/tunnel"
)

// onConnect 由 transport.ReconnectClient 在每次 TLS 连接建立后回调。
func (e *Engine) onConnect(conn *transport.Conn) {
	e.setState(StateConnecting)
	e.mu.Lock()
	creds := e.creds
	e.mu.Unlock()

	hs := tunnel.NewClientHandshake()
	user := strings.TrimSpace(creds.Username)
	pass := creds.Password
	if user == "" || pass == "" {
		logger.Warn("隧道握手失败: 缺少账号密码")
		conn.Close()
		return
	}
	hsStart := time.Now()
	hsRes, err := hs.RunAuthWithTimeout(conn, user, pass, 20*time.Second)
	if err != nil {
		logger.Warn("隧道握手失败: %v elapsed=%s", err, time.Since(hsStart))
		conn.Close()
		return
	}
	logger.Info("隧道鉴权应答收到 elapsed=%s", time.Since(hsStart))

	priv := strings.TrimSpace(hsRes.ClientPrivateKey)
	if priv == "" {
		priv = strings.TrimSpace(creds.PrivateKey)
	}
	if priv == "" {
		logger.Warn("握手未下发私钥且无内存回退私钥")
		conn.Close()
		return
	}
	sess, err := tunnel.BuildClientCrypto(priv, hsRes.ServerPublicKey)
	if err != nil {
		logger.Warn("建立加密会话失败: %v", err)
		conn.Close()
		return
	}

	conn.SetOnData(func(data []byte) {
		plain, err := sess.Decrypt(data)
		if err != nil {
			return
		}
		_ = e.rt.write(plain)
	})
	conn.SetOnClose(func(error) {
		e.activeMu.Lock()
		defer e.activeMu.Unlock()
		if e.activeConn != conn {
			return
		}
		e.activeConn = nil
		e.cryptoSess = nil
		logger.Info("隧道连接已断开，等待重连")
		e.protectThenClearRoutes()
		e.mu.Lock()
		e.state = StateReconnecting
		e.mu.Unlock()
	})
	e.activeMu.Lock()
	e.cryptoSess = sess
	e.activeConn = conn
	e.sessionPriv = priv
	e.activeMu.Unlock()

	policyStart := time.Now()
	if err := e.rt.applyPolicy(hsRes.Policy); err != nil {
		logger.Warn("应用服务端策略失败: %v elapsed=%s", err, time.Since(policyStart))
		e.protectThenClearRoutes()
		conn.Close()
		return
	}

	e.activeMu.Lock()
	stillActive := e.activeConn == conn && conn.State() == transport.StateConnected
	if !stillActive {
		if e.activeConn == conn {
			e.activeConn = nil
			e.cryptoSess = nil
		}
		e.activeMu.Unlock()
		logger.Warn("session_abandoned reason=disconnected_during_policy")
		e.protectThenClearRoutes()
		return
	}
	mtu := netutil.ResolveMTU(hsRes.Policy.MTU, e.cfg.Tun.MTU)
	e.mu.Lock()
	e.vpnIP = hsRes.Policy.VPNIP
	e.gateway = hsRes.Policy.GatewayIP
	e.state = StateConnected
	e.mu.Unlock()
	e.activeMu.Unlock()

	if e.cfg.Security.KillSwitch {
		if err := e.ks.Disable(); err != nil {
			logger.Error("杀开关拆除失败: %v", err)
			e.setKillSwitchStatus(false, fmt.Sprintf("杀开关拆除失败: %v", err))
		} else {
			e.setKillSwitchStatus(true, "")
		}
	}
	logger.Info("隧道握手成功 vpn_ip=%s policy_ver=%d gateway=%s mtu=%d policy_elapsed=%s",
		hsRes.Policy.VPNIP, hsRes.Policy.PolicyVer, hsRes.Policy.GatewayIP, mtu, time.Since(policyStart))
}

func (e *Engine) tunReadLoop(ctx context.Context) {
	mtu := netutil.ResolveMTU(e.cfg.Tun.MTU)
	e.rt.readLoop(ctx, func(b []byte) error {
		e.activeMu.Lock()
		conn := e.activeConn
		sess := e.cryptoSess
		e.activeMu.Unlock()
		if conn == nil || sess == nil {
			return nil
		}
		if conn.State() != transport.StateConnected {
			return nil
		}
		enc, err := sess.Encrypt(b)
		if err != nil {
			return err
		}
		return conn.Send(enc)
	}, mtu)
}
