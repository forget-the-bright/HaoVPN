package api

import (
	"net/http"

	"haovpn/internal/auth"
	"haovpn/internal/logger"
)

// requireAuth 鉴权中间件，包装需登录的 API 处理器。
//
// 未登录返回 401 JSON；用户已删/禁用时吊销会话并 401（失败关闭）。
// MustChangePassword 时仅放行 /api/v1/password 与 /api/v1/logout；查询失败亦 401。
// POST/PUT/PATCH/DELETE 须通过 validateCSRF；GET 豁免 CSRF。
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		se, ok := s.sessionFromRequest(r)
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "未登录")
			return
		}
		if err := s.auth.UserActiveForSession(se.UserID); err != nil {
			s.invalidateRequestSession(r)
			writeAPIError(w, http.StatusUnauthorized, "会话已失效")
			return
		}
		mustChange, err := s.auth.MustChangePassword(se.UserID)
		if err != nil {
			s.invalidateRequestSession(r)
			writeAPIError(w, http.StatusUnauthorized, "会话已失效")
			return
		}
		if mustChange {
			allowed := r.URL.Path == "/api/v1/password" || r.URL.Path == "/api/v1/logout"
			if !allowed {
				writeAPIError(w, http.StatusForbidden, "须先修改密码")
				return
			}
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			if !s.validateCSRF(r) {
				writeAPIError(w, http.StatusForbidden, "CSRF token 无效")
				return
			}
		}
		next(w, r)
	}
}

// requireAuthPage WebUI 页面鉴权中间件。
//
// 未登录、用户失效或须改密时重定向 /login；不返回 JSON，供 HTML 页面路由使用。
func (s *Server) requireAuthPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		se, ok := s.sessionFromRequest(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if err := s.auth.UserActiveForSession(se.UserID); err != nil {
			s.invalidateRequestSession(r)
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		mustChange, err := s.auth.MustChangePassword(se.UserID)
		if err != nil || mustChange {
			if err != nil {
				s.invalidateRequestSession(r)
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// invalidateRequestSession 吊销当前请求 Cookie 对应的 Web 会话。
func (s *Server) invalidateRequestSession(r *http.Request) {
	if c, err := r.Cookie("session"); err == nil {
		s.auth.Logout(c.Value)
	}
}

// sessionFromRequest 从请求 Cookie 解析并校验 Web 会话。
//
// 返回：会话条目与是否有效；Cookie 缺失或 token 过期时 ok=false。
func (s *Server) sessionFromRequest(r *http.Request) (auth.SessionEntry, bool) {
	c, err := r.Cookie("session")
	if err != nil {
		return auth.SessionEntry{}, false
	}
	return s.auth.ValidateSession(c.Value)
}

// validateCSRF 校验写操作的 CSRF Token。
//
// Token 优先取 X-CSRF-Token 头，其次解析表单 csrf_token；与会话 Cookie 配对校验。
func (s *Server) validateCSRF(r *http.Request) bool {
	c, err := r.Cookie("session")
	if err != nil {
		return false
	}
	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		if err := parseRequestForm(r); err != nil {
			return false
		}
		token = r.FormValue("csrf_token")
	}
	return s.auth.ValidateCSRF(c.Value, token)
}

// handleLogin 处理登录请求（POST /api/v1/login；页面表单亦指向此路由）。
//
// 成功时设置 session Cookie、返回 csrf_token 与 must_change_password；失败写审计 login_failed。
// 副作用：写入会话 Cookie（HttpOnly、SameSite=Lax）；登录接口豁免 CSRF；顺带 prune 过期会话。
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	if err := parseRequestForm(r); err != nil {
		ip := s.clientIP(r)
		logger.Warn("登录表单解析失败 ip=%s ct=%q err=%v", ip, r.Header.Get("Content-Type"), err)
		writeAPIError(w, http.StatusBadRequest, "invalid form data")
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	ip := s.clientIP(r)
	token, user, err := s.auth.Login(username, password, ip)
	if err != nil {
		s.audit.Log(nil, "login_failed", "user", nil, ip, map[string]string{"username": username})
		writeAPIError(w, http.StatusUnauthorized, err.Error())
		return
	}
	_ = s.auth.PruneExpiredSessions()
	csrf := s.auth.GetCSRF(token)
	cookie := &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.cfg.API.SessionTTLSec,
	}
	if s.cfg.API.SecureCookies || r.TLS != nil {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
	s.audit.Log(&user.ID, "login", "user", &user.ID, ip, nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"must_change_password": user.MustChangePassword,
		"csrf_token":           csrf,
	})
}

// handleLogout 注销当前 Web 会话（POST /api/v1/logout）。
//
// 副作用：销毁服务端会话、清除 session Cookie、写审计 logout。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("session"); err == nil {
		se, ok := s.auth.ValidateSession(c.Value)
		if ok {
			uid := se.UserID
			s.audit.Log(&uid, "logout", "user", &uid, s.clientIP(r), nil)
		}
		s.auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
	writeOK(w)
}

// handleChangePassword 修改当前登录用户密码（POST /api/v1/password）。
//
// 参数：表单 old_password、new_password；成功后清除 MustChangePassword，并吊销该用户全部 Web 会话。
// 副作用：更新 DB 密码哈希、写审计 change_password；前端须重新登录。
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	se, ok := s.sessionFromRequest(r)
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "未登录")
		return
	}
	if err := parseRequestForm(r); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid form data")
		return
	}
	oldPass := r.FormValue("old_password")
	newPass := r.FormValue("new_password")
	if oldPass == "" {
		writeAPIError(w, http.StatusBadRequest, "请填写当前密码")
		return
	}
	if err := s.auth.ChangePassword(se.UserID, oldPass, newPass); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	n := s.auth.LogoutAllForUser(se.UserID)
	logger.Info("用户改密成功并吊销 Web 会话 user_id=%d sessions=%d", se.UserID, n)
	s.audit.Log(&se.UserID, "change_password", "user", &se.UserID, s.clientIP(r), nil)
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "relogin": true})
}

// handleCSRF 返回当前会话的 CSRF Token（GET /api/v1/csrf）。
//
// 未登录或会话无效时返回 401；供 WebUI 写操作前刷新 token。
// 经 sessionFromRequest 校验会话，直接使用 SessionEntry.CSRFToken（与登录路径一致）。
func (s *Server) handleCSRF(w http.ResponseWriter, r *http.Request) {
	se, ok := s.sessionFromRequest(r)
	if !ok || se.CSRFToken == "" {
		writeAPIError(w, http.StatusUnauthorized, "未登录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": se.CSRFToken})
}
