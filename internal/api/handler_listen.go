package api

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/safeutil"
)

// newAdminHTTPServer 构造管理面 http.Server，统一超时以防 Slowloris / 悬挂连接。
//
// ReadHeaderTimeout 10s；Read/Write 60s；Idle 120s。与 StartAllListeners / Listen 共用。
func newAdminHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// StartAllListeners 在多个 host 上并发启动管理 API 监听。
//
// 参数：hosts — api.listen_hosts（含 TUN IP 追加后）；port — 管理端口。
// 返回：成功 Listen 的 *http.Server 切片（可能少于 hosts）；全部失败时为空切片并打 Error。
// 行为：本机地址优先绑定；非 loopback 失败时最多重试 8 次（等 TUN 就绪）；经 safeutil.GoSafe 服务。
// 关联：serverapp 启动后调用；FormatBoundAddrs 用于日志。
func StartAllListeners(s *Server, hosts []string, port int) []*http.Server {
	var ordered []string
	var rest []string
	for _, h := range hosts {
		if netutil.IsLoopbackHost(h) {
			ordered = append(ordered, h)
		} else {
			rest = append(rest, h)
		}
	}
	ordered = append(ordered, rest...)

	var servers []*http.Server
	handler := s.withMiddleware(s.mux)
	for _, host := range ordered {
		addr := fmt.Sprintf("%s:%d", host, port)
		retries := 1
		if !netutil.IsLoopbackHost(host) {
			retries = 8
		}
		ln, err := listenAPI(addr, retries)
		if err != nil {
			logger.Warn("管理 API 跳过 %s: %v（本机请用 127.0.0.1:%d）", addr, err, port)
			continue
		}
		srv := newAdminHTTPServer(addr, handler)
		servers = append(servers, srv)
		safeutil.GoSafe("api-listen-"+addr, func() {
			logger.Info("管理 API 已监听: %s", ln.Addr().String())
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				logger.Error("管理 API 错误 %s: %v", srv.Addr, err)
			}
		})
	}
	if len(servers) == 0 {
		logger.Error("管理 API 无可用监听地址，请检查 api.listen_hosts")
	}
	return servers
}

// FormatBoundAddrs 将已监听地址格式化为逗号分隔字符串。
//
// 参数：servers — StartAllListeners 返回值；空时返回「(无)」。
// 用途：serverapp 启动日志展示实际绑定地址（与未使用的预格式化列表区分）。
func FormatBoundAddrs(servers []*http.Server) string {
	var s string
	for i, srv := range servers {
		if i > 0 {
			s += ", "
		}
		s += srv.Addr
	}
	if s == "" {
		return "(无)"
	}
	return s
}

// listenAPI 在 addr 上 Listen，失败时按 retries 间隔 300ms 重试。
//
// 参数：retries < 1 时按 1 次；非 loopback 场景用于等待 TUN IP 就绪。
// 实现委托 safeutil.RetryN，保持日志埋点。
func listenAPI(addr string, retries int) (net.Listener, error) {
	if retries < 1 {
		retries = 1
	}
	var ln net.Listener
	attempt := 0
	err := safeutil.RetryN(retries, 300*time.Millisecond, func() error {
		attempt++
		var e error
		ln, e = net.Listen("tcp", addr)
		if e != nil && attempt < retries {
			logger.Info("管理 API bind 重试 %s (%d/%d): %v", addr, attempt, retries, e)
		}
		return e
	})
	if err != nil {
		return nil, err
	}
	return ln, nil
}
