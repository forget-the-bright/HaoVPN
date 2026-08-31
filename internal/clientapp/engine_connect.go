package clientapp

import (
	"context"
	"errors"
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
// 鉴权成功后：先写入 vpnIP 等展示字段，再 signalFirstResult(nil)，使 GUI/托盘可显示分配 IP；
// TUN/路由在后台继续配置，完成后才置 StateConnected / connectedAt。
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
		e.reportFirstFailure(errors.New(msg), true)
		return
	}
	hsStart := time.Now()
	lans := netutil.ValidLANCIDRs(e.cfg.LocalLANs)
	hsRes, err := hs.RunAuthWithTimeoutEx(conn, user, pass, lans, clientHostID(), 20*time.Second)
	if err != nil {
		logger.Warn("隧道握手失败: %v elapsed=%s", err, time.Since(hsStart))
		conn.Close()
		// 保留哨兵 identity，供 WaitConnected / fatal 判定 errors.Is
		e.reportFirstFailure(err, e.ShouldFailFastHandshake(err))
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
		e.reportFirstFailure(errors.New(msg), true)
		return
	}
	sess, err := tunnel.BuildClientCrypto(priv, hsRes.ServerPublicKey)
	if err != nil {
		logger.Warn("建立加密会话失败: %v", err)
		conn.Close()
		e.reportFirstFailure(fmt.Errorf("建立加密会话失败: %w", err), true)
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
		e.connectedAt = time.Time{}
		e.mu.Unlock()
	})
	e.activeMu.Lock()
	e.cryptoSess = sess
	e.activeConn = conn
	e.sessionPriv = priv
	e.activeMu.Unlock()

	// 鉴权已通过：先写入展示用会话字段（主窗/托盘可显示 VPN IP），再 signal；
	// StateConnected / connectedAt 仍等 applyPolicy 成功后再置（数据面就绪）。
	e.mu.Lock()
	e.vpnIP = hsRes.Policy.VPNIP
	e.gateway = hsRes.Policy.GatewayIP
	e.vpnSubnet = strings.TrimSpace(hsRes.Policy.VPNSubnet)
	e.managedRoutes = ManagedRoutesFromTunnel(hsRes.Policy.ManagedRoutes)
	e.allowedIPs = append([]string{}, hsRes.Policy.AllowedIPs...)
	e.mu.Unlock()

	e.markAuthOK()
	e.signalFirstResult(nil)
	logger.Info("鉴权成功，正在配置 TUN/路由… vpn_ip=%s", hsRes.Policy.VPNIP)

	policyStart := time.Now()
	conn.SetHeartbeatTimeoutPaused(true)
	err = e.rt.applyPolicy(hsRes.Policy)
	conn.SetHeartbeatTimeoutPaused(false)
	if err != nil {
		logger.Warn("应用服务端策略失败: %v elapsed=%s", err, time.Since(policyStart))
		e.dataplaneFailed(conn, fmt.Sprintf("应用服务端策略失败: %v", err))
		return
	}
	routeWarn := e.rt.takeRouteWarn()

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
	// vpnIP 等已在鉴权后写入；此处确认并打点连接时刻
	e.vpnIP = hsRes.Policy.VPNIP
	e.gateway = hsRes.Policy.GatewayIP
	e.vpnSubnet = strings.TrimSpace(hsRes.Policy.VPNSubnet)
	e.managedRoutes = ManagedRoutesFromTunnel(hsRes.Policy.ManagedRoutes)
	e.allowedIPs = append([]string{}, hsRes.Policy.AllowedIPs...)
	e.state = StateConnected
	e.connectedAt = time.Now()
	// 部分分流失败保留提示；否则清空旧错误
	e.lastError = routeWarn
	e.mu.Unlock()
	e.activeMu.Unlock()

	if routeWarn != "" {
		logger.Warn("partial_routes=true connected_with_warn: %s", routeWarn)
	}
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

// onDialError 拨号/TLS 失败回调。
//
// 未鉴权成功前：通知 WaitConnected（GUI failFast）。
// 已上线后：普通错误仅记日志由重连继续；致命拨号错误（封禁/源拒绝/明文拒绝）
// 须置 Idle + LastError，因 ReconnectClient 已自行停 loop，否则 GUI 会卡在「重连中」。
func (e *Engine) onDialError(err error) {
	if err == nil {
		return
	}
	msg := FormatDialError(err)
	logger.Warn("%s", msg)
	fatal := IsFatalDialError(err)
	if e.hasAuthOKOnce() {
		if fatal {
			e.setLastError(msg)
			e.setState(StateIdle)
			e.stopReconnectOnly()
			logger.Warn("已停止自动重连（致命拨号错误）: %s", msg)
		}
		return
	}
	// UX 文案作 Error() 前缀，同时 %w 保留拨号哨兵供 errors.Is
	e.reportFirstFailure(fmt.Errorf("%s: %w", msg, err), fatal)
}
