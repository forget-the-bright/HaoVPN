package api

import (
	"net/http"

	"haovpn/internal/auth"
	"haovpn/internal/logger"
)

// requireAuth 鉴权中间件，包装需登录的 API 处理器。
//
// 未登录返回 401 JSON；MustChangePassword 时仅放行 /api/v1/password 与 /api/v1/logout。
// POST/PUT/PATCH/DELETE 须通过 validateCSRF；GET 豁免 CSRF。
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

// requireAuthPage WebUI 页面鉴权中间件。
//
// 未登录或须改密时重定向 /login；不返回 JSON，供 HTML 页面路由使用。
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
		_ = parseRequestForm(r)
		token = r.FormValue("csrf_token")
	}
	return s.auth.ValidateCSRF(c.Value, token)
}

// handleLogin 处理登录请求（POST /api/v1/login；页面表单亦指向此路由）。
//
// 成功时设置 session Cookie、返回 csrf_token 与 must_change_password；失败写审计 login_failed。
// 副作用：写入会话 Cookie（HttpOnly、SameSite=Lax）；登录接口豁免 CSRF。
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

// handleLogout 注销当前 Web 会话（POST /api/v1/logout）。
//
// 副作用：销毁服务端会话、清除 session Cookie、写审计 logout。
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

// handleChangePassword 修改当前登录用户密码（POST /api/v1/password）。
//
// 参数：表单 new_password；成功后清除 MustChangePassword 标记。
// 副作用：更新 DB 密码哈希、写审计 change_password。
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
