package clientapp

import (
	"context"
	"fmt"
	"sync"

	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
	"haovpn/internal/security"
	"haovpn/internal/transport"
)

// protectThenClearRoutes 断线防护：先装杀开关再清路由。
func (e *Engine) protectThenClearRoutes() {
	if e.cfg.Security.KillSwitch {
		prefixes := e.rt.allowedIPs()
		if len(prefixes) == 0 {
			e.setKillSwitchStatus(false, "杀开关启用失败: AllowedIPs 为空，已保留路由以防泄漏")
			logger.Error("杀开关启用失败: AllowedIPs 为空，禁止清路由")
			return
		}
		if err := e.ks.Enable(prefixes); err != nil {
			e.setKillSwitchStatus(false, fmt.Sprintf("杀开关启用失败: %v（已保留路由，工控流量仍走 TUN）", err))
			logger.Error("杀开关启用失败，禁止清路由: %v", err)
			return
		}
		e.setKillSwitchStatus(true, "")
	}
	if e.clearRoutesHook != nil {
		e.clearRoutesHook()
	}
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
	e.state = StateConnecting
	e.lastError = ""
	// 每次 Start 重置首次结果通道，供 WaitConnected 等待本次连接
	e.firstResultOnce = sync.Once{}
	e.firstResultCh = make(chan error, 1)
	e.mu.Unlock()

	tcfg := transport.FromClientConfig(e.cfg)
	e.reconnect = transport.NewReconnectClient(e.cfg.Server.Address, tlsCfg, tcfg, nil, e.onConnect)
	e.reconnect.SetOnDialError(e.onDialError)
	e.reconnect.Start()

	safeutil.GoSafe("client-tun-read", func() {
		e.tunReadLoop(ctx)
	})
	return nil
}

// Stop 停止重连循环、关闭 TUN/路由并拆除杀开关。
func (e *Engine) Stop() {
	e.activeMu.Lock()
	e.activeConn = nil
	e.cryptoSess = nil
	e.sessionPriv = ""
	e.activeMu.Unlock()

	e.mu.Lock()
	cancel := e.cancel
	e.cancel = nil
	rc := e.reconnect
	e.reconnect = nil
	e.state = StateIdle
	e.vpnIP = ""
	e.mu.Unlock()

	if rc != nil {
		rc.Stop()
	}
	if cancel != nil {
		cancel()
	}
	e.rt.close()
	if err := e.ks.Remove(); err != nil {
		logger.Error("拆除杀开关失败: %v", err)
		e.setKillSwitchStatus(false, fmt.Sprintf("拆除杀开关失败: %v", err))
	} else {
		e.setKillSwitchStatus(true, "")
	}
}
