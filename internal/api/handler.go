package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/health"
	"haovpn/internal/logstore"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
	"haovpn/internal/readmodel"
	"haovpn/internal/safeutil"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/version"
	"haovpn/internal/vpnaccount"
)

// Server 管理 Web API 与页面路由的 HTTP 服务实例，聚合鉴权、审计、VPN 账号与在线会话等模块。
//
// 字段：
//   cfg — 服务端全局配置（监听地址、会话 TTL、数据库路径等）；构造时注入，生命周期内只读引用。
//   store — SQLite 持久化层；读写用户、审计、连接事件等。
//   auth — Web 登录会话与 CSRF 校验服务。
//   audit — 管理操作审计日志写入器。
//   sessions — VPN 隧道在线会话管理器；踢线、在线数统计、监控页数据源。
//   keyEnc — 账号私钥加解密；配置导出与 provisioning 时使用。
//   vpnSvc — VPN 账号业务（IP 分配、策略、删除）；须非 nil 方可创建/删除/PATCH 账号。
//   startedAt — 进程启动时刻；health/dashboard 计算 uptime。
//   serverPK — 服务端隧道公钥（Base64）；导出客户端 ZIP/YAML 时写入 peer 配置。
//   mux — 路由表；routes 注册完成后不变。
//   httpServer — Listen 单地址模式下的 http.Server；Close 时优雅关闭。
//   tunOK — 数据面 TUN 是否就绪；由 SetDataplaneHealth 在启动完成后注入。
//   natOK — 数据面 NAT 是否就绪；由 SetDataplaneHealth 在启动完成后注入。
//   logStore — 结构化历史日志库（可选）；未注入时 /api/v1/logs?source=history 返回空列表。
//
// 线程安全：HTTP 处理器并发调用；依赖 store/auth/sessions 等下游各自的并发安全。
type Server struct {
	cfg        *config.ServerConfig
	store      *persist.Store
	auth       *auth.Service
	audit      *audit.Logger
	sessions   *sessionmgr.Manager
	keyEnc     *security.KeyEnc
	vpnSvc     *vpnaccount.Service
	startedAt  time.Time
	serverPK   string
	mux        *http.ServeMux
	httpServer *http.Server
	tunOK      bool
	natOK      bool
	logStore   *logstore.Store
}

// NewServer 创建 API 服务实例并完成路由注册。
//
// 参数：
//   cfg — 服务端配置；非 nil。
//   store — 持久化存储；非 nil。
//   authSvc — Web 鉴权服务；非 nil。
//   auditLog — 审计日志；非 nil。
//   sessMgr — VPN 在线会话管理器；非 nil。
//   vpnSvc — VPN 账号领域服务；非 nil。
//   keyEnc — 私钥加解密；可为 nil（仅影响需解密私钥的导出/provisioning）。
//   startedAt — 服务启动时间戳；用于 uptime。
//   serverTunnelPublicKey — 隧道公钥字符串；写入客户端导出配置。
//
// 返回：已注册全部路由的 *Server；调用方可继续 SetLogStore/SetVPNService/SetDataplaneHealth。
func NewServer(
	cfg *config.ServerConfig,
	store *persist.Store,
	authSvc *auth.Service,
	auditLog *audit.Logger,
	sessMgr *sessionmgr.Manager,
	vpnSvc *vpnaccount.Service,
	keyEnc *security.KeyEnc,
	startedAt time.Time,
	serverTunnelPublicKey string,
) *Server {
	s := &Server{
		cfg:       cfg,
		store:     store,
		auth:      authSvc,
		audit:     auditLog,
		sessions:  sessMgr,
		vpnSvc:    vpnSvc,
		keyEnc:    keyEnc,
		startedAt: startedAt,
		serverPK:  serverTunnelPublicKey,
		mux:       http.NewServeMux(),
	}
	s.routes()
	return s
}

// SetLogStore 注入结构化历史日志库（可选）。
//
// 未注入时 /api/v1/logs?source=history 返回空列表；可在 NewServer 之后调用。
func (s *Server) SetLogStore(ls *logstore.Store) {
	s.logStore = ls
}

// SetVPNService 注入或替换 VPN 账号领域服务。
//
// 用于 IP 模式/策略解析与开户；须在 handleUsers 等写操作前注入非 nil 实例。
func (s *Server) SetVPNService(v *vpnaccount.Service) {
	s.vpnSvc = v
}

// SetDataplaneHealth 设置 TUN/NAT 数据面就绪状态。
//
// 由 serverapp 在 TUN 与 NAT 配置完成后注入；供 health/dashboard 展示。
func (s *Server) SetDataplaneHealth(tunOK, natOK bool) {
	s.tunOK = tunOK
	s.natOK = natOK
}

// routes 注册所有 HTTP 路由，按三组划分：
//
//   公开接口 — 无需登录：/api/v1/login、/login、/api/v1/health、/api/v1/system/info。
//   需登录 API — requireAuth 鉴权 + CSRF（POST/PUT/PATCH/DELETE）：用户/审计/仪表盘/备份/日志/改密/CSRF/监控等 /api/v1/*。
//   WebUI 页面 — go:embed 模板：/static/、/、/users、/connections、/audit、/tools；/peers 重定向至 /users。
func (s *Server) routes() {
	// 公开接口
	s.mux.HandleFunc("/api/v1/login", s.handleLogin) // 登录接口豁免 CSRF
	s.mux.HandleFunc("/login", s.handleLoginPage)
	s.mux.HandleFunc("/api/v1/health", s.handleHealth)
	s.mux.HandleFunc("/api/v1/system/info", s.handleSystemInfo)

	// 需登录
	s.mux.HandleFunc("/api/v1/logout", s.requireAuth(s.handleLogout))
	s.mux.HandleFunc("/api/v1/users", s.requireAuth(s.handleUsers))
	s.mux.HandleFunc("/api/v1/users/", s.requireAuth(s.handleUserByID))
	s.mux.HandleFunc("/api/v1/audit", s.requireAuth(s.handleAudit))
	s.mux.HandleFunc("/api/v1/dashboard", s.requireAuth(s.handleDashboard))
	s.mux.HandleFunc("/api/v1/backup", s.requireAuth(s.handleBackup))
	s.mux.HandleFunc("/api/v1/logs", s.requireAuth(s.handleLogs))
	s.mux.HandleFunc("/api/v1/password", s.requireAuth(s.handleChangePassword))
	s.mux.HandleFunc("/api/v1/csrf", s.requireAuth(s.handleCSRF))
	s.mux.HandleFunc("/api/v1/monitor/online", s.requireAuth(s.handleMonitorOnline))
	s.mux.HandleFunc("/api/v1/monitor/accounts", s.requireAuth(s.handleMonitorAccounts))
	s.mux.HandleFunc("/api/v1/monitor/events", s.requireAuth(s.handleMonitorEvents))

	// WebUI 页面（go:embed 模板）
	s.mux.HandleFunc("/static/", s.handleStatic)
	s.mux.HandleFunc("/", s.handleDashboardPage)
	s.mux.HandleFunc("/users", s.requireAuthPage(s.handleUsersPage))
	s.mux.HandleFunc("/peers", s.requireAuthPage(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/users", http.StatusFound)
	}))
	s.mux.HandleFunc("/connections", s.requireAuthPage(s.handleConnectionsPage))
	s.mux.HandleFunc("/audit", s.requireAuthPage(s.handleAuditPage))
	s.mux.HandleFunc("/tools", s.requireAuthPage(s.handleToolsPage))
}

// Handler 返回带安全中间件包装的 HTTP 处理器。
//
// 供 httptest 与自定义监听使用；等价于 Listen 内部使用的 handler 链。
func (s *Server) Handler() http.Handler {
	return s.withMiddleware(s.mux)
}

// Listen 在指定地址启动单监听 HTTP 服务。
//
// 返回：ListenAndServe 错误（正常关闭时为 http.ErrServerClosed）。
// 副作用：创建 s.httpServer；Close 可优雅关停。
func (s *Server) Listen(addr string) error {
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.withMiddleware(s.mux),
	}
	logger.Info("管理 API 监听: %s", addr)
	return s.httpServer.ListenAndServe()
}

// Close 优雅关闭由 Listen 创建的 HTTP 服务。
//
// 未调用 Listen 或已关闭时返回 nil。
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Close()
}

// withMiddleware 为路由链注入安全响应头（CSP 等）。
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range security.SecurityHeaders() {
			w.Header().Set(k, v)
		}
		next.ServeHTTP(w, r)
	})
}

// handleHealth 健康检查（GET /api/v1/health，公开）。
//
// 返回：uptime、在线数、db/tun/nat 状态及近期错误摘要。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbOK := s.store.DB().Ping() == nil
	st := health.NewStatus(s.startedAt, s.sessions.OnlineCount(), dbOK, s.tunOK, s.natOK, logger.RecentErrors())
	writeJSON(w, http.StatusOK, st)
}

// handleSystemInfo 返回构建版本信息（GET /api/v1/system/info，公开）。
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.Info())
}

// handleAudit 分页查询管理审计日志（GET /api/v1/audit）。
//
// 查询参数：action、since（RFC3339）、limit、offset。
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := clampLimit(parseIntDefault(q.Get("limit"), 50), 50, 500)
	offset := parseIntDefault(q.Get("offset"), 0)
	var since time.Time
	if t := strings.TrimSpace(q.Get("since")); t != "" {
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			since = parsed
		}
	}
	logs, total, err := s.store.ListAuditLogsFiltered(readmodel.AuditListFilter{
		Action: q.Get("action"),
		Since:  since,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": logs, "total": total, "limit": limit, "offset": offset,
	})
}

// handleDashboard 仪表盘摘要 JSON（GET /api/v1/dashboard）。
//
// 返回：在线账号数、uptime、db/tun/nat 状态、recent_errors。
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	dbOK := s.store.DB().Ping() == nil
	writeJSON(w, http.StatusOK, map[string]any{
		"online_accounts": s.sessions.OnlineCount(),
		"online_peers":    s.sessions.OnlineCount(), // 兼容旧 dashboard JS
		"uptime_sec":      int64(time.Since(s.startedAt).Seconds()),
		"db_ok":           dbOK,
		"tun_ok":          s.tunOK,
		"nat_ok":          s.natOK,
		"recent_errors":   logger.RecentErrors(),
	})
}

// handleLogs 读取实时或历史日志（GET /api/v1/logs）。
//
// 参数：source=live|history（默认 live）、tail、level、q、since、offset。
// history 走 logStore；live 读 server.log 或 *.live.log 尾部。
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	q := r.URL.Query()
	source := strings.ToLower(strings.TrimSpace(q.Get("source")))
	if source == "" {
		source = "live"
	}
	tail := clampLimit(parseLogTailQuery(q.Get("tail")), 200, 2000)

	switch source {
	case "history":
		if s.logStore == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"source": "history", "items": []any{}, "total": 0, "lines": []string{},
			})
			return
		}
		limit := tail
		offset := parseIntDefault(q.Get("offset"), 0)
		var since time.Time
		if t := strings.TrimSpace(q.Get("since")); t != "" {
			if parsed, err := time.Parse(time.RFC3339, t); err == nil {
				since = parsed
			}
		}
		items, total, err := s.logStore.Query(logstore.Query{
			Level: q.Get("level"), Keyword: q.Get("q"), Since: since,
			Limit: limit, Offset: offset,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var lines []string
		for _, it := range items {
			lines = append(lines, it.Line)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"source": "history", "items": items, "lines": lines,
			"total": total, "limit": limit, "offset": offset,
			"file": s.cfg.ResolveHistoryDBPath(),
		})
	default:
		path := s.cfg.Log.File
		if source == "live" {
			if lp := logger.LivePath(); lp != "" {
				path = lp
			}
		}
		lines, truncated, err := readLogTail(path, tail)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"source": source, "lines": lines, "truncated": truncated, "file": path,
		})
	}
}

// handleBackup 下载 SQLite 主库备份（GET /api/v1/backup）。
//
// 副作用：写审计 db_backup；直接 ServeFile 数据库路径。
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	se, _ := s.sessionFromRequest(r)
	s.audit.Log(&se.UserID, "db_backup", "system", nil, clientIP(r), nil)
	http.ServeFile(w, r, s.cfg.Database.Path)
}

// LogPublicBindAudit 公网绑定启动时写审计记录。
//
// 当 allow_public_bind 开启时由 serverapp 调用，提示管理口暴露风险。
func LogPublicBindAudit(auditLog *audit.Logger) {
	auditLog.Log(nil, "management_public_bind_enabled", "system", nil, "", map[string]string{
		"message": "用户已开启 allow_public_bind，管理口暴露风险自担",
	})
}

// StartAllListeners 在多个 host 上并发启动管理 API 监听。
//
// 先绑回环地址再绑 TUN IP，避免 TUN 重试拖慢本机 WebUI；非回环地址 bind 最多重试 8 次。
// 返回：已成功创建的 http.Server 切片（可能为空）。
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
		srv := &http.Server{Addr: addr, Handler: handler}
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
// 无可用监听时返回 "(无)"。
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

// listenAPI 尝试 TCP 绑定指定地址。
//
// 参数：retries>1 时用于 TUN IP 刚配置后的短暂重试窗口（间隔 300ms）。
// 返回：成功时 Listener；全部失败时 last error。
func listenAPI(addr string, retries int) (net.Listener, error) {
	if retries < 1 {
		retries = 1
	}
	var last error
	for i := 0; i < retries; i++ {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
		last = err
		if i+1 < retries {
			logger.Info("管理 API bind 重试 %s (%d/%d): %v", addr, i+1, retries, err)
			time.Sleep(300 * time.Millisecond)
		}
	}
	return nil, last
}
