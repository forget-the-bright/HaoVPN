package clientapp

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
)

// ErrHardRestartAborted GUI 在 Stop/DNS 间隙请求取消（退出登录/退出/新一轮重连抢占）。
var ErrHardRestartAborted = errors.New("hard restart aborted")

// waitDNSReady 无 abort 的 DNS settle（包内单测用）。
func waitDNSReady(addr string, timeout time.Duration) bool {
	return waitDNSReadyAbort(addr, timeout, nil)
}

// waitDNSReadyAbort 在 settle 轮询中等待 DNS 可用；abort() 为 true 时立即返回 false。
//
// 参数：
//   addr — host:port 或裸主机名；
//   timeout — 总等待上限（手动重连约 3s）；
//   abort — 可选；每轮 Lookup 前检查，true 则中止 settle（HardRestart 退出登录/抢占）。
func waitDNSReadyAbort(addr string, timeout time.Duration, abort func() bool) bool {
	host := strings.TrimSpace(addr)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" || net.ParseIP(host) != nil {
		return true // IP 直连无需 DNS
	}
	start := time.Now()
	deadline := start.Add(timeout)
	ok := safeutil.PollUntil(deadline, 200*time.Millisecond, abort, func() bool {
		_, err := net.LookupHost(host)
		return err == nil
	})
	elapsed := time.Since(start)
	if ok {
		logger.Info("reconnect_dns_settle elapsed=%s ok=true host=%s", elapsed, host)
		return true
	}
	if abort != nil && abort() {
		logger.Info("reconnect_dns_settle elapsed=%s ok=false aborted=true host=%s", elapsed, host)
		return false
	}
	logger.Warn("reconnect_dns_settle elapsed=%s ok=false host=%s err=timeout", elapsed, host)
	return false
}

// HardRestart 手动重连：StopKeepICS（ICSPreserve）→ DNS settle → 新 Engine 拨号。
//
// 与 Soft 重连对比：
//   - Soft：transport.ReconnectClient + protectForReconnect（保留 TUN/路由/DNS/via）
//   - Hard：清数据面但 ICSPreserve；有 137 则下次 /24 + reuse_live（跳过 Restart/Enable）
//
// 参数：
//   old — 可为 nil（无旧引擎时仅 settle+新建）；非 nil 时先 StopKeepICS。
//   cfg — 客户端配置（须非 nil，取 Server.Address 做 settle）。
//   creds — 隧道凭据。
//   abort — 可选；在 Stop 后、DNS settle 轮询中、DNS 后、Start 前若返回 true，则返回 ErrHardRestartAborted（eng 为 nil）。
// 返回：新 Engine（Start 已调用）；Start 失败时仍返回 eng 非 nil 便于调用方 Stop，error 非 nil。
// FailFast：固定 false（Stop 后 DNS 窗口常致首次 lookup timeout，应退避而非卡死登录态）。
// 登录页首次连接仍自行 NewEngine + SetFailFast(true)，勿走本函数。
func HardRestart(old *Engine, cfg *config.ClientConfig, creds Credentials, abort func() bool) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("HardRestart: 配置为空")
	}
	aborted := func() bool {
		return abort != nil && abort()
	}
	start := time.Now()
	logger.Info("hard_restart begin")
	if old != nil {
		old.StopKeepICS()
	}
	if aborted() {
		logger.Info("hard_restart aborted after_stop elapsed=%s", time.Since(start))
		return nil, ErrHardRestartAborted
	}
	// settle 中亦可 abort，避免退出登录卡满约 3s DNS 窗口
	waitDNSReadyAbort(cfg.Server.Address, 3*time.Second, abort)
	if aborted() {
		logger.Info("hard_restart aborted after_dns elapsed=%s", time.Since(start))
		return nil, ErrHardRestartAborted
	}
	eng := NewEngine(cfg)
	eng.SetCredentials(creds)
	// 勿 SetFailFast(true)：与登录页语义分离（见函数注释）。
	if aborted() {
		logger.Info("hard_restart aborted before_start elapsed=%s", time.Since(start))
		return nil, ErrHardRestartAborted
	}
	if err := eng.Start(); err != nil {
		logger.Warn("hard_restart start_fail elapsed=%s err=%v", time.Since(start), err)
		return eng, err
	}
	logger.Info("hard_restart done elapsed=%s", time.Since(start))
	return eng, nil
}
