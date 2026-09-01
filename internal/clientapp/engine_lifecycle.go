package clientapp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
	"haovpn/internal/security"
	"haovpn/internal/transport"
)

// enableKillSwitchIfConfigured 断线时若开启杀开关则 Enable；失败不拆数据面。
//
// 返回 ok=false 表示 Enable 失败（调用方不得 clearRoutes，以防工控流量回落物理网卡）。
func (e *Engine) enableKillSwitchIfConfigured() (ok bool) {
	if !e.cfg.Security.KillSwitch {
		return true
	}
	prefixes := e.rt.allowedIPs()
	if len(prefixes) == 0 {
		e.setKillSwitchStatus(false, "杀开关启用失败: AllowedIPs 为空，已保留路由以防泄漏")
		logger.Error("杀开关启用失败: AllowedIPs 为空，禁止清路由")
		return false
	}
	if err := e.ks.Enable(prefixes); err != nil {
		e.setKillSwitchStatus(false, fmt.Sprintf("杀开关启用失败: %v（已保留路由，工控流量仍走 TUN）", err))
		logger.Error("杀开关启用失败，禁止清路由: %v", err)
		return false
	}
	e.setKillSwitchStatus(true, "")
	return true
}

// protectForReconnect 临时断线防护：仅启杀开关（若配置），保留 TUN/路由/via/DNS。
//
// 供自动重连路径使用；握手后 applyPolicy 按差分增删，配置未变则 noop。
func (e *Engine) protectForReconnect() {
	_ = e.enableKillSwitchIfConfigured()
	logger.Info("dataplane_keep reason=reconnect")
}

// protectThenClearRoutes 全量防护：先装杀开关再清路由（Stop 失败路径 / dataplaneFailed）。
func (e *Engine) protectThenClearRoutes() {
	if !e.enableKillSwitchIfConfigured() {
		return
	}
	if e.clearRoutesHook != nil {
		e.clearRoutesHook()
	}
	logger.Info("dataplane_clear reason=dataplane_failed_or_explicit")
	e.rt.clearRoutes()
}

func (e *Engine) setKillSwitchStatus(ok bool, userErr string) {
	e.mu.Lock()
	e.ksOK = ok
	e.lastError = userErr
	e.mu.Unlock()
}

// Start 在后台启动 TLS 重连循环与 TUN 读循环。
func (e *Engine) Start() error {
	if e.cfg.Security.KillSwitch {
		if err := e.ks.Supported(); err != nil {
			return fmt.Errorf("杀开关: %w", err)
		}
	}
	tlsCfg, err := security.BuildClientTLS(e.cfg)
	if err != nil {
		return err
	}
	e.mu.Lock()
	if e.cancel != nil {
		e.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.runCtx = ctx
	e.cancel = cancel
	e.stopping = false
	e.state = StateConnecting
	e.lastError = ""
	// 每次 Start 重置首次结果通道，供 WaitConnected 等待本次连接
	e.firstResultOnce = sync.Once{}
	e.firstResultCh = make(chan error, 1)
	e.mu.Unlock()

	tcfg := transport.FromClientConfig(e.cfg)
	logger.Info("transport send_queue_size=%d", tcfg.MaxQueueSize)
	e.reconnect = transport.NewReconnectClient(e.cfg.Server.Address, tlsCfg, tcfg, nil, e.onConnect)
	e.reconnect.SetOnDialError(e.onDialError)
	e.reconnect.Start()

	safeutil.GoSafe("client-tun-read", func() {
		e.tunReadLoop(ctx)
	})
	return nil
}

// Stop 停止重连循环、关闭 TUN/路由并拆除杀开关（并关闭 ICS）。
func (e *Engine) Stop() {
	e.stop(false)
}

// StopKeepICS HardRestart 专用：清数据面但保留 Windows ICS（有 137 则下次秒级复用）。
func (e *Engine) StopKeepICS() {
	e.stop(true)
}

// stop keepICS=true 时 via TeardownKeepICS；Logout 等全清须 keepICS=false。
//
// 顺序关键（日志实测）：须先 cancel runCtx，再关 Conn/等 loop。
// 旧顺序先 rc.Stop（关 Conn 并等 onConnect）后 cancel → applyPolicy 仍跑完 ICS（十余秒），
// 再走 session_abandoned soft 重连，与 HardRestart 观感「清理和拨号并行」。
func (e *Engine) stop(keepICS bool) {
	e.mu.Lock()
	e.stopping = true
	cancel := e.cancel
	e.cancel = nil
	rc := e.reconnect
	e.reconnect = nil
	e.state = StateIdle
	e.vpnIP = ""
	e.gateway = ""
	e.vpnSubnet = ""
	e.managedRoutes = nil
	e.allowedIPs = nil
	e.connectedAt = time.Time{}
	e.mu.Unlock()

	// 先取消：applyPolicy 在 via/ICS 前检查 ctx，可跳过昂贵 Setup
	if cancel != nil {
		cancel()
	}
	logger.Info("engine_stop begin keep_ics=%v", keepICS)

	e.activeMu.Lock()
	e.activeConn = nil
	e.cryptoSess = nil
	e.sessionPriv = ""
	e.activeMu.Unlock()

	if rc != nil {
		// 须等 loop 退出：否则手动重连 NewEngine.Start 会与旧 Dial/onConnect 竞态。
		rc.Stop()
	}
	// 传输已停后再清数据面（DNS/路由/TUN），禁止并行僵尸 Dial。
	e.rt.close(keepICS)
	if err := e.ks.Remove(); err != nil {
		logger.Error("拆除杀开关失败: %v", err)
		e.setKillSwitchStatus(false, fmt.Sprintf("拆除杀开关失败: %v", err))
	} else {
		e.setKillSwitchStatus(true, "")
	}
	logger.Info("engine_stop done")
}
