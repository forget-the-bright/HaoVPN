package clientapp

import (
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
)

// WaitDNSReady Stop 清数据面后短等服务器主机名可解析，缓解 VPN DNS 残留窗口。
//
// 参数：addr — host:port 或裸主机名；timeout — 总等待上限（手动重连约 3s）。
// 返回：超时前 LookupHost 成功为 true；失败仍应继续拨号（交给传输层退避）。
// 须在后台 goroutine 调用，禁止在 UI 线程阻塞。
// 实现：按 200ms 间隔用 safeutil.RetryN 轮询（无 cancel 的短 settle；可中断 sleep 仍在 transport）。
// 日志键：reconnect_dns_settle（与历史 GUI 埋点一致，便于现场检索）。
func WaitDNSReady(addr string, timeout time.Duration) bool {
	host := strings.TrimSpace(addr)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" || net.ParseIP(host) != nil {
		return true // IP 直连无需 DNS
	}
	start := time.Now()
	delay := 200 * time.Millisecond
	attempts := int(timeout / delay)
	if attempts < 1 {
		attempts = 1
	}
	err := safeutil.RetryN(attempts, delay, func() error {
		_, e := net.LookupHost(host)
		return e
	})
	if err == nil {
		logger.Info("reconnect_dns_settle elapsed=%s ok=true host=%s", time.Since(start), host)
		return true
	}
	logger.Warn("reconnect_dns_settle elapsed=%s ok=false host=%s err=%v", time.Since(start), host, err)
	return false
}

// HardRestart 手动全量重连契约：Stop（等 reconnect loop）→ DNS settle → 新 Engine 拨号。
//
// 与 Soft 重连对比：
//   - Soft：transport.ReconnectClient + protectForReconnect（保留 TUN/路由/DNS/via）
//   - Hard：本函数（全清数据面后再拨）；GUI/CLI 禁止再发明第三套编排
//
// 参数：
//   old — 可为 nil（无旧引擎时仅 settle+新建）；非 nil 时先 Stop。
//   cfg — 客户端配置（须非 nil，取 Server.Address 做 settle）。
//   creds — 隧道凭据。
// 返回：新 Engine（Start 已调用）；Start 失败时仍返回 eng 非 nil 便于调用方 Stop，error 非 nil。
// FailFast：固定 false（Stop 后 DNS 窗口常致首次 lookup timeout，应退避而非卡死登录态）。
// 登录页首次连接仍自行 NewEngine + SetFailFast(true)，勿走本函数。
func HardRestart(old *Engine, cfg *config.ClientConfig, creds Credentials) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("HardRestart: 配置为空")
	}
	start := time.Now()
	logger.Info("hard_restart begin")
	if old != nil {
		old.Stop()
	}
	WaitDNSReady(cfg.Server.Address, 3*time.Second)
	eng := NewEngine(cfg)
	eng.SetCredentials(creds)
	// 勿 SetFailFast(true)：与登录页语义分离（见函数注释）。
	if err := eng.Start(); err != nil {
		logger.Warn("hard_restart start_fail elapsed=%s err=%v", time.Since(start), err)
		return eng, err
	}
	logger.Info("hard_restart done elapsed=%s", time.Since(start))
	return eng, nil
}
