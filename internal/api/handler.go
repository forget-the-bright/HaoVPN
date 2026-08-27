// Package api 提供 HTTP 管理 API 与 WebUI 路由（默认仅本机 + TUN IP）。
package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/health"
	"haovpn/internal/ippool"
	"haovpn/internal/logstore"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/safeutil"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/version"
	"haovpn/internal/vpnaccount"
)

// Server 管理 API 服务，聚合各业务模块。
type Server struct {
	cfg        *config.ServerConfig
	store      *persist.Store
	auth       *auth.Service
	audit      *audit.Logger
	sessions   *sessionmgr.Manager
	ipPool     *ippool.Pool
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

// NewServer 创建 API 服务实例。
func NewServer(
	cfg *config.ServerConfig,
	store *persist.Store,
	authSvc *auth.Service,
	auditLog *audit.Logger,
	sessMgr *sessionmgr.Manager,
	pool *ippool.Pool,
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
		ipPool:    pool,
		keyEnc:    keyEnc,
		vpnSvc:    nil, // main 注入
		startedAt: startedAt,
		serverPK:  serverTunnelPublicKey,
		mux:       http.NewServeMux(),
	}
	s.routes()
	return s
}

// SetLogStore 注入结构化历史日志库（可选）。
func (s *Server) SetLogStore(ls *logstore.Store) {
	s.logStore = ls
}

// SetVPNService 注入 VPN 账号服务（IP 模式/策略解析）。
func (s *Server) SetVPNService(v *vpnaccount.Service) {
	s.vpnSvc = v
}

// SetDataplaneHealth 设置 TUN/NAT 运行态（启动完成后由 main 注入）。
func (s *Server) SetDataplaneHealth(tunOK, natOK bool) {
	s.tunOK = tunOK
	s.natOK = natOK
}

// routes 注册所有 HTTP 路由。
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

// Handler 返回 HTTP 处理器（测试与自定义监听用）。
func (s *Server) Handler() http.Handler {
	return s.withMiddleware(s.mux)
}

// Listen 在指定地址启动 HTTP 服务。
func (s *Server) Listen(addr string) error {
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.withMiddleware(s.mux),
	}
	logger.Info("管理 API 监听: %s", addr)
	return s.httpServer.ListenAndServe()
}

// Close 优雅关闭 HTTP 服务。
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Close()
}

// withMiddleware 包装安全响应头与日志。
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range security.SecurityHeaders() {
			w.Header().Set(k, v)
		}
		next.ServeHTTP(w, r)
	})
}

// requireAuth 鉴权中间件：未登录返回 401；须改密时仅允许改密/登出。
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		se, ok := s.sessionFromRequest(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录"})
			return
		}
		if u, err := s.store.GetUserByID(se.UserID); err == nil && u.MustChangePassword {
			allowed := r.URL.Path == "/api/v1/password" || r.URL.Path == "/api/v1/logout"
			if !allowed {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "须先修改密码"})
				return
			}
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			if !s.validateCSRF(r) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "CSRF token 无效"})
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) requireAuthPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		se, ok := s.sessionFromRequest(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if u, err := s.store.GetUserByID(se.UserID); err == nil && u.MustChangePassword {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func (s *Server) sessionFromRequest(r *http.Request) (auth.SessionEntry, bool) {
	c, err := r.Cookie("session")
	if err != nil {
		return auth.SessionEntry{}, false
	}
	return s.auth.ValidateSession(c.Value)
}

func (s *Server) validateCSRF(r *http.Request) bool {
	c, err := r.Cookie("session")
	if err != nil {
		return false
	}
	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		_ = parseRequestForm(r)
		token = r.FormValue("csrf_token")
	}
	return s.auth.ValidateCSRF(c.Value, token)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbOK := s.store.DB().Ping() == nil
	st := health.NewStatus(s.startedAt, s.sessions.OnlineCount(), dbOK, s.tunOK, s.natOK, logger.RecentErrors())
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.Info())
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	if err := parseRequestForm(r); err != nil {
		ip := clientIP(r)
		logger.Warn("登录表单解析失败 ip=%s ct=%q err=%v", ip, r.Header.Get("Content-Type"), err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid form data"})
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	ip := clientIP(r)
	token, user, err := s.auth.Login(username, password, ip)
	if err != nil {
		s.audit.Log(nil, "login_failed", "user", nil, ip, map[string]string{"username": username})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	csrf := s.auth.GetCSRF(token)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.cfg.API.SessionTTLSec,
	})
	s.audit.Log(&user.ID, "login", "user", &user.ID, ip, nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"must_change_password": user.MustChangePassword,
		"csrf_token":           csrf,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("session"); err == nil {
		se, ok := s.auth.ValidateSession(c.Value)
		if ok {
			uid := se.UserID
			s.audit.Log(&uid, "logout", "user", &uid, clientIP(r), nil)
		}
		s.auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	se, ok := s.sessionFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, nil)
		return
	}
	_ = parseRequestForm(r)
	newPass := r.FormValue("new_password")
	hash, err := auth.HashPassword(newPass)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.UpdateUserPassword(se.UserID, hash, true); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "更新失败"})
		return
	}
	s.audit.Log(&se.UserID, "change_password", "user", &se.UserID, clientIP(r), nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		limit := clampLimit(parseIntDefault(q.Get("limit"), 50), 50, 500)
		offset := parseIntDefault(q.Get("offset"), 0)
		enabled := -1
		useEnabled := false
		if v := q.Get("enabled"); v == "1" || v == "true" {
			enabled = 1
			useEnabled = true
		} else if v == "0" || v == "false" {
			enabled = 0
			useEnabled = true
		}
		items, total, err := s.store.ListUsersPage(persist.UserListFilter{
			Q: q.Get("q"), Enabled: enabled, UseEnabled: useEnabled, Limit: limit, Offset: offset,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		onlineOnly := q.Get("online") == "1" || q.Get("online") == "true"
		type accountView struct {
			ID         int64    `json:"id"`
			Username   string   `json:"username"`
			Enabled    bool     `json:"enabled"`
			HasVPN     bool     `json:"has_vpn"`
			VPNIP      string   `json:"vpn_ip,omitempty"`
			IPMode     string   `json:"ip_mode,omitempty"`
			PolicyVer  int      `json:"policy_ver,omitempty"`
			AllowedIPs []string `json:"allowed_ips,omitempty"`
			Online     bool     `json:"online"`
		}
		var out []accountView
		for _, u := range items {
			_, online := s.sessions.GetSession(u.ID)
			if onlineOnly && !online {
				continue
			}
			out = append(out, accountView{
				ID: u.ID, Username: u.Username, Enabled: u.Enabled,
				HasVPN: u.HasVPN, VPNIP: u.VPNIP, IPMode: u.IPMode,
				PolicyVer: u.PolicyVer, AllowedIPs: u.AllowedIPs, Online: online,
			})
		}
		if onlineOnly {
			total = len(out)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": out, "total": total, "limit": limit, "offset": offset,
		})
	case http.MethodPost:
		_ = parseRequestForm(r)
		username := r.FormValue("username")
		password := r.FormValue("password")
		ipMode := r.FormValue("ip_mode")
		if ipMode == "" {
			ipMode = persist.IPModeFixed
		}
		ipLeaseSec, _ := strconv.Atoi(r.FormValue("ip_lease_sec"))
		requestedIP := strings.TrimSpace(r.FormValue("vpn_ip"))
		hash, err := auth.HashPassword(password)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		id, vpnIP, err := s.provisionVPNAccount(username, hash, ipMode, ipLeaseSec, nil, requestedIP)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		se, _ := s.sessionFromRequest(r)
		s.audit.Log(&se.UserID, "account_create", "user", &id, clientIP(r), map[string]string{"username": username, "vpn_ip": vpnIP})
		writeJSON(w, http.StatusOK, map[string]any{
			"id": id, "username": username, "vpn_ip": vpnIP, "ip_mode": ipMode,
			"policy_ver": 1, "export_zip_url": fmt.Sprintf("/api/v1/users/%d/export.zip", id),
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleUserByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	parts := strings.Split(path, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 ID"})
		return
	}
	se, _ := s.sessionFromRequest(r)

	// GET /api/v1/users/{id}/export.zip
	if len(parts) > 1 && parts[1] == "export.zip" && r.Method == http.MethodGet {
		s.handleUserExportZip(w, r, id, se)
		return
	}
	if len(parts) > 1 && parts[1] == "export" && r.Method == http.MethodGet {
		s.handleUserExportYAML(w, r, id, se)
		return
	}
	if len(parts) > 1 && parts[1] == "kick" && r.Method == http.MethodPost {
		s.sessions.KickUser(id)
		s.audit.Log(&se.UserID, "kick_account", "user", &id, clientIP(r), nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) > 1 && parts[1] == "vpn" && r.Method == http.MethodPatch {
		s.handleUserVPNPatch(w, r, id, se)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		u, _ := s.store.GetUserByID(id)
		s.sessions.KickUser(id)
		if u != nil && u.VPNIP != "" {
			// fixed / 动态残留占用都释放内存池；DB 行由 DeleteUser 级联删
			s.ipPool.Release(u.VPNIP)
			s.sessions.UnregisterVPNIP(u.VPNIP)
		}
		if err := s.store.DeleteUser(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.audit.Log(&se.UserID, "account_delete", "user", &id, clientIP(r), nil)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodPost:
		_ = parseRequestForm(r)
		action := r.FormValue("action")
		if action == "disable" {
			if err := s.store.SetUserEnabled(id, false); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			s.sessions.KickUser(id)
			logger.Info("账号已禁用并踢线: user_id=%d", id)
			s.audit.Log(&se.UserID, "user_disable", "user", &id, clientIP(r), nil)
		} else if action == "enable" {
			if err := s.store.SetUserEnabled(id, true); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			s.audit.Log(&se.UserID, "user_enable", "user", &id, clientIP(r), nil)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, nil)
	}
}

func (s *Server) handleUserVPNPatch(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	var body struct {
		AllowedIPs *[]string `json:"allowed_ips"`
		IPMode     string    `json:"ip_mode"`
		IPLeaseSec int       `json:"ip_lease_sec"`
		VPNIP      *string   `json:"vpn_ip"` // nil=不改；非 nil 时按模式处理
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 JSON"})
		return
	}
	u, err := s.store.GetUserByID(id)
	if err != nil || !u.HasVPN() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "账号不存在或无 VPN 身份"})
		return
	}
	oldMode, oldIP := u.IPMode, u.VPNIP
	if oldMode == "" {
		oldMode = persist.IPModeFixed
	}
	newMode := oldMode
	if body.IPMode != "" {
		newMode = body.IPMode
	}
	if body.IPLeaseSec > 0 {
		u.IPLeaseSec = body.IPLeaseSec
	}
	allowed := u.AllowedIPs
	if body.AllowedIPs != nil {
		if err := validateAllowedIPs(*body.AllowedIPs); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		allowed = *body.AllowedIPs
	}

	newIP := oldIP
	if body.VPNIP != nil {
		req := strings.TrimSpace(*body.VPNIP)
		switch newMode {
		case persist.IPModeDynamicSession, persist.IPModeDynamicLease:
			if req != "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "动态 IP 模式不可指定 VPN IP"})
				return
			}
			newIP = ""
		case persist.IPModeFixed:
			if req == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "fixed 模式须指定 VPN IP，或省略 vpn_ip 字段保持不变"})
				return
			}
			newIP = req
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "未知 ip_mode"})
			return
		}
	} else if newMode != oldMode {
		// 仅改模式、未带 vpn_ip
		if newMode == persist.IPModeFixed && oldIP == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "切到 fixed 须指定 vpn_ip"})
			return
		}
		if newMode == persist.IPModeDynamicSession || newMode == persist.IPModeDynamicLease {
			newIP = ""
		}
	}

	// 池占用调整
	wasFixed := oldMode == persist.IPModeFixed
	willFixed := newMode == persist.IPModeFixed
	if wasFixed && !willFixed {
		s.releaseFixedVPNIP(id, oldIP)
	} else if willFixed {
		if !wasFixed {
			// 从动态切 fixed：占用指定 IP
			if err := s.rebindFixedVPNIP(id, "", newIP); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		} else if newIP != oldIP {
			if err := s.rebindFixedVPNIP(id, oldIP, newIP); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
		if p := net.ParseIP(newIP); p != nil && p.To4() != nil {
			newIP = p.To4().String()
		}
	}

	pv, err := s.store.IncrementPolicyVer(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.UpdateVPNFields(id, newIP, allowed, newMode, u.IPLeaseSec, pv); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.sessions.KickUser(id)
	s.audit.Log(&se.UserID, "policy_change_kick", "user", &id, clientIP(r), map[string]string{
		"policy_ver": fmt.Sprintf("%d", pv), "vpn_ip": newIP, "ip_mode": newMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "policy_ver": pv, "vpn_ip": newIP, "ip_mode": newMode})
}

func (s *Server) handleUserExportZip(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	u, err := s.store.GetUserByID(id)
	if err != nil || !u.HasVPN() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "账号不存在或无 VPN 配置"})
		return
	}
	plainKey, err := s.openAccountPrivateKey(u)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "私钥解密失败"})
		return
	}
	zipBytes, err := buildAccountExportZip(s.cfg, u, plainKey, s.serverPK)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.audit.Log(&se.UserID, "config_export", "user", &id, clientIP(r), map[string]string{"format": "zip"})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=haovpn-client-%s.zip", u.Username))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(zipBytes)
}

func (s *Server) handleUserExportYAML(w http.ResponseWriter, r *http.Request, id int64, se auth.SessionEntry) {
	u, err := s.store.GetUserByID(id)
	if err != nil || !u.HasVPN() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "账号不存在或无 VPN 配置"})
		return
	}
	plainKey, err := s.openAccountPrivateKey(u)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "私钥解密失败"})
		return
	}
	caFile := s.cfg.Server.TLS.CertFile
	if caFile == "" {
		caFile = "./certs/server.crt"
	}
	yaml := buildClientExportYAML(s.cfg, u, plainKey, s.serverPK, caFile)
	s.audit.Log(&se.UserID, "config_export", "user", &id, clientIP(r), nil)
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=client-%s.yaml", u.Username))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(yaml))
}

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
	logs, total, err := s.store.ListAuditLogsFiltered(persist.AuditListFilter{
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

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	se, _ := s.sessionFromRequest(r)
	s.audit.Log(&se.UserID, "db_backup", "system", nil, clientIP(r), nil)
	http.ServeFile(w, r, s.cfg.Database.Path)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.Split(x, ",")[0]
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

// LogPublicBindAudit 公网绑定启动时写审计记录。
func LogPublicBindAudit(auditLog *audit.Logger) {
	auditLog.Log(nil, "management_public_bind_enabled", "system", nil, "", map[string]string{
		"message": "用户已开启 allow_public_bind，管理口暴露风险自担",
	})
}

// StartAllListeners 在多个 host 上启动管理 API。
// 先绑回环再绑 TUN IP，避免 TUN 重试拖慢本机 WebUI 就绪。
func StartAllListeners(s *Server, hosts []string, port int) []*http.Server {
	var ordered []string
	var rest []string
	for _, h := range hosts {
		if h == "127.0.0.1" || h == "::1" || h == "localhost" {
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
		if host != "127.0.0.1" && host != "::1" && host != "localhost" {
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

// FormatBoundAddrs 仅列出已成功监听的地址。
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

// listenAPI 尝试绑定；retries>1 用于 TUN IP 刚配置后的短暂窗口。
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
