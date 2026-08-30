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
//
// 鉴权成功后立即 signalFirstResult(nil)，使 GUI WaitConnected 可先进入主界面；
// TUN/路由在后台继续配置，完成后才置 StateConnected。
func (e *Engine) onConnect(conn *transport.Conn) {
	e.setState(StateConnecting)
	e.mu.Lock()
	creds := e.creds
	e.mu.Unlock()

	hs := tunnel.NewClientHandshake()
	user := strings.TrimSpace(creds.Username)
	pass := creds.Password
	if user == "" || pass == "" {
		msg := "缺少账号密码"
		logger.Warn("隧道握手失败: %s", msg)
		conn.Close()
		e.reportFirstFailure(msg, true)
		return
	}
	hsStart := time.Now()
	lans := netutil.ValidLANCIDRs(e.cfg.LocalLANs)
	hsRes, err := hs.RunAuthWithTimeoutEx(conn, user, pass, lans, clientHostID(), 20*time.Second)
	if err != nil {
		logger.Warn("隧道握手失败: %v elapsed=%s", err, time.Since(hsStart))
		conn.Close()
		// 首次失败必须通知 WaitConnected（含握手超时）；GUI failFast 会停重连
		e.reportFirstFailure(err.Error(), e.ShouldFailFastHandshake(err))
		return
	}
	logger.Info("隧道鉴权应答收到 elapsed=%s", time.Since(hsStart))
	e.resetOnlineRejects()

	priv := strings.TrimSpace(hsRes.ClientPrivateKey)
	if priv == "" {
		priv = strings.TrimSpace(creds.PrivateKey)
	}
	if priv == "" {
		msg := "握手未下发私钥且无内存回退私钥"
		logger.Warn("%s", msg)
		conn.Close()
		e.reportFirstFailure(msg, true)
		return
	}
	sess, err := tunnel.BuildClientCrypto(priv, hsRes.ServerPublicKey)
	if err != nil {
		logger.Warn("建立加密会话失败: %v", err)
		conn.Close()
		e.reportFirstFailure(fmt.Sprintf("建立加密会话失败: %v", err), true)
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
		// 临时断线保留 TUN/路由/ICS，握手后差分；仅启杀开关（若配置）
		e.protectForReconnect()
		e.mu.Lock()
		e.state = StateReconnecting
		e.mu.Unlock()
	})
	e.activeMu.Lock()
	e.cryptoSess = sess
	e.activeConn = conn
	e.sessionPriv = priv
	e.activeMu.Unlock()

	// 鉴权已通过：关闭登录 failFast，唤醒 WaitConnected，后台继续配 TUN/路由
	e.markAuthOK()
	e.signalFirstResult(nil)
	logger.Info("鉴权成功，正在配置 TUN/路由…")

	policyStart := time.Now()
	if err := e.rt.applyPolicy(hsRes.Policy); err != nil {
		logger.Warn("应用服务端策略失败: %v elapsed=%s", err, time.Since(policyStart))
		e.dataplaneFailed(conn, fmt.Sprintf("应用服务端策略失败: %v", err))
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
		logger.Warn("session_abandoned reason=disconnected_during_policy，等待自动重连")
		e.protectForReconnect()
		e.setLastError("连接在配置网络时断开，正在重连…")
		e.setState(StateReconnecting)
		// 不停重连循环：瞬时断线交给 ReconnectClient 下一轮 Dial
		return
	}
	mtu := netutil.ResolveMTU(hsRes.Policy.MTU, e.cfg.Tun.MTU)
	e.mu.Lock()
	e.vpnIP = hsRes.Policy.VPNIP
	e.gateway = hsRes.Policy.GatewayIP
	e.vpnSubnet = strings.TrimSpace(hsRes.Policy.VPNSubnet)
	e.managedRoutes = append([]tunnel.ManagedRoute{}, hsRes.Policy.ManagedRoutes...)
	e.allowedIPs = append([]string{}, hsRes.Policy.AllowedIPs...)
	e.state = StateConnected
	e.lastError = ""
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

// dataplaneFailed 鉴权已成功并通知 GUI 后，TUN/路由失败时的收尾。
//
// 停重连、清数据面，并触发 OnDataplaneFailed（GUI 回登录红字）。
func (e *Engine) dataplaneFailed(conn *transport.Conn, msg string) {
	e.setLastError(msg)
	e.setState(StateIdle)
	e.protectThenClearRoutes()
	if conn != nil {
		conn.Close()
	}
	e.stopReconnectOnly()
	if fn := e.dataplaneFailedCallback(); fn != nil {
		fn(msg)
	}
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

// onDialError 拨号/TLS 失败回调：未鉴权成功前通知 WaitConnected（GUI failFast 可停）；
// 已上线后仅记日志，由 ReconnectClient 继续退避重拨。
func (e *Engine) onDialError(err error) {
	if err == nil {
		return
	}
	msg := fmt.Sprintf("无法连接服务器: %v", err)
	logger.Warn("%s", msg)
	if e.hasAuthOKOnce() {
		return
	}
	e.reportFirstFailure(msg, false)
}
