package api

import "net/http"

// routes 注册所有 HTTP 路由（公开 / 需登录 API / WebUI 页面）。
func (s *Server) routes() {
	s.mux.HandleFunc("/api/v1/login", s.handleLogin)
	s.mux.HandleFunc("/login", s.handleLoginPage)
	s.mux.HandleFunc("/api/v1/health", s.handleHealth)
	s.mux.HandleFunc("/api/v1/system/info", s.handleSystemInfo)

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
	s.mux.HandleFunc("/api/v1/security/events", s.requireAuth(s.handleSecurityEvents))
	s.mux.HandleFunc("/api/v1/security/blocks", s.requireAuth(s.handleSecurityBlocks))
	s.mux.HandleFunc("/api/v1/security/blocks/", s.requireAuth(s.handleSecurityBlockByIP))

	s.mux.HandleFunc("/static/", s.handleStatic)
	s.mux.HandleFunc("/", s.handleDashboardPage)
	s.mux.HandleFunc("/users", s.requireAuthPage(s.handleUsersPage))
	s.mux.HandleFunc("/peers", s.requireAuthPage(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/users", http.StatusFound)
	}))
	s.mux.HandleFunc("/connections", s.requireAuthPage(s.handleConnectionsPage))
	s.mux.HandleFunc("/audit", s.requireAuthPage(s.handleAuditPage))
	s.mux.HandleFunc("/security", s.requireAuthPage(s.handleSecurityPage))
	s.mux.HandleFunc("/tools", s.requireAuthPage(s.handleToolsPage))
}
