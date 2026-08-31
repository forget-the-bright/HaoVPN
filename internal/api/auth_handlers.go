package api

import (
	"net/http"

	"haovpn/internal/auth"
	"haovpn/internal/logger"
	"haovpn/internal/security"
)

// webSessionStatus validateWebSession 的校验结果。
type webSessionStatus int

const (
	webSessionOK webSessionStatus = iota
	webSessionUnauthorized
	webSessionInvalid
	webSessionMustChangePassword
)

// validateWebSession 校验 Cookie/Context 会话：存在、用户有效、须改密策略。
//
// allowMustChangeBypass 为 true 时（requireAuth）允许改密/登出/CSRF 路径；Page 中间件传 false。
func (s *Server) validateWebSession(r *http.Request, allowMustChangeBypass bool) (auth.SessionEntry, webSessionStatus) {
	se, ok := s.sessionFromRequest(r)
	if !ok {
		return auth.SessionEntry{}, webSessionUnauthorized
	}
	if err := s.auth.UserActiveForSession(se.UserID); err != nil {
		s.invalidateRequestSession(r)
		return auth.SessionEntry{}, webSessionInvalid
	}
	mustChange, err := s.auth.MustChangePassword(se.UserID)
	if err != nil {
		s.invalidateRequestSession(r)
		return auth.SessionEntry{}, webSessionInvalid
	}
	if mustChange {
		if allowMustChangeBypass {
			allowed := r.URL.Path == "/api/v1/password" ||
				r.URL.Path == "/api/v1/logout" ||
				(r.Method == http.MethodGet && r.URL.Path == "/api/v1/csrf")
			if !allowed {
				return se, webSessionMustChangePassword
			}
		} else {
			return se, webSessionMustChangePassword
		}
	}
	return se, webSessionOK
}

// requireAuth 鉴权中间件，包装需登录的 API 处理器。
//
// 未登录返回 401 JSON；用户已删/禁用/非管理员时吊销会话并 401（失败关闭）。
// MustChangePassword 时仅放行 /api/v1/password、/api/v1/logout 与 GET /api/v1/csrf
// （须改密流程刷新 CSRF，避免登录后丢内存 token 无法再拉）；查询失败亦 401。
// 鉴权成功后 TouchSession 并重发 session Cookie（刷新浏览器 MaxAge，与服务端滑动续期对齐；
// 绝对上限仍由 auth.Session.AbsoluteExpiresAt 约束）。
// POST/PUT/PATCH/DELETE 须通过 validateCSRF；GET 豁免 CSRF（敏感下载已改为 POST）。
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		se, st := s.validateWebSession(r, true)
		switch st {
		case webSessionUnauthorized:
			writeAPIError(w, http.StatusUnauthorized, "未登录")
			return
		case webSessionInvalid:
			writeAPIError(w, http.StatusUnauthorized, "会话已失效")
			return
		case webSessionMustChangePassword:
			writeAPIError(w, http.StatusForbidden, "须先修改密码")
			return
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			if !s.validateCSRF(r) {
				writeAPIError(w, http.StatusForbidden, "CSRF token 无效")
				return
			}
		}
		if c, err := r.Cookie("session"); err == nil {
			s.auth.TouchSession(c.Value)
			s.setSessionCookie(w, r, c.Value)
		}
		next(w, withRequestSession(r, se))
	}
}

// sessionCookieSecure 是否应对 session Cookie 置 Secure。
// 配置 api.secure_cookies、直连 TLS，或可信反代 X-Forwarded-Proto: https 时为 true。
func (s *Server) sessionCookieSecure(r *http.Request) bool {
	return security.RequestIsHTTPS(r, s.cfg.API.SecureCookies, s.cfg.API.TrustedProxyCIDRs)
}

// setSessionCookie 写入/刷新 session Cookie（登录与 Touch 滑动续期共用）。
//
// 属性：Path=/、HttpOnly、SameSite=Lax、MaxAge=SessionTTLSec；Secure 见 sessionCookieSecure。
// 清除时必须用 clearSessionCookie 保持相同属性，否则 HTTPS 下浏览器可能删不掉旧 Cookie。
func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	cookie := &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.cfg.API.SessionTTLSec,
	}
	if s.sessionCookieSecure(r) {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
}

// clearSessionCookie 清除 session Cookie，属性与 setSessionCookie 对齐（含 Secure/SameSite）。
//
// 现代浏览器删除 Cookie 时要求 Path/Secure/SameSite 与设置时一致，否则 logout 后会话仍存活。
func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	cookie := &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
	if s.sessionCookieSecure(r) {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
}

// requireAuthPage WebUI 页面鉴权中间件。
//
// 未登录、用户失效或须改密时重定向 /login；不返回 JSON，供 HTML 页面路由使用。
func (s *Server) requireAuthPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		se, st := s.validateWebSession(r, false)
		if st != webSessionOK {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, withRequestSession(r, se))
	}
}

// invalidateRequestSession 吊销当前请求 Cookie 对应的 Web 会话。
func (s *Server) invalidateRequestSession(r *http.Request) {
	if c, err := r.Cookie("session"); err == nil {
		s.auth.Logout(c.Value)
	}
}

// sessionFromRequest 从 request.Context（middleware 注入）或 Cookie 解析 Web 会话。
func (s *Server) sessionFromRequest(r *http.Request) (auth.SessionEntry, bool) {
	if se, ok := sessionFromContext(r); ok {
		return se, true
	}
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
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !parseFormOrError(w, r) {
		ip := s.clientIP(r)
		logger.Warn("登录表单解析失败 ip=%s ct=%q", ip, r.Header.Get("Content-Type"))
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	ip := s.clientIP(r)
	token, user, err := s.auth.Login(username, password, ip)
	if err != nil {
		s.audit.Log(nil, "login_failed", "user", nil, ip, map[string]string{"username": username})
		// 对外统一文案，避免锁定/错密 oracle；细节仅日志与审计
		logger.Warn("登录失败 ip=%s user=%q err=%v", ip, username, err)
		writeAPIError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	_ = s.auth.PruneExpiredSessions()
	csrf := s.auth.GetCSRF(token)
	s.setSessionCookie(w, r, token)
	s.audit.Log(&user.ID, "login", "user", &user.ID, ip, nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"must_change_password": user.MustChangePassword,
		"csrf_token":           csrf,
	})
}

// handleLogout 注销当前 Web 会话（仅 POST /api/v1/logout；须 CSRF，禁止 GET 防跨站注销）。
//
// 副作用：销毁服务端会话、清除 session Cookie、写审计 logout。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if c, err := r.Cookie("session"); err == nil {
		se, ok := s.auth.ValidateSession(c.Value)
		if ok {
			uid := se.UserID
			s.audit.Log(&uid, "logout", "user", &uid, s.clientIP(r), nil)
		}
		s.auth.Logout(c.Value)
	}
	s.clearSessionCookie(w, r)
	writeOK(w)
}

// handleChangePassword 修改当前登录用户密码（POST /api/v1/password）。
//
// 参数：表单 old_password、new_password；成功后清除 MustChangePassword，并吊销该用户全部 Web 会话。
// 副作用：更新 DB 密码哈希、写审计 change_password；前端须重新登录。
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	se, _ := sessionFromContext(r)
	if se.UserID == 0 {
		writeAPIError(w, http.StatusUnauthorized, "未登录")
		return
	}
	if !parseFormOrError(w, r) {
		return
	}
	oldPass := r.FormValue("old_password")
	newPass := r.FormValue("new_password")
	if oldPass == "" {
		writeAPIError(w, http.StatusBadRequest, "请填写当前密码")
		return
	}
	if err := s.auth.ChangePassword(se.UserID, oldPass, newPass); err != nil {
		writeDomainError(w, err)
		return
	}
	n := s.auth.LogoutAllForUser(se.UserID)
	logger.Info("用户改密成功并吊销 Web 会话 user_id=%d sessions=%d", se.UserID, n)
	s.audit.Log(&se.UserID, "change_password", "user", &se.UserID, s.clientIP(r), nil)
	s.clearSessionCookie(w, r)
	writeOKWith(w, map[string]any{"relogin": true})
}

// handleCSRF 返回当前会话的 CSRF Token（GET /api/v1/csrf）。
//
// 未登录或会话无效时返回 401；供 WebUI 写操作前刷新 token。
// 经 sessionFromRequest 校验会话，直接使用 SessionEntry.CSRFToken（与登录路径一致）。
func (s *Server) handleCSRF(w http.ResponseWriter, r *http.Request) {
	se, ok := sessionFromContext(r)
	if !ok || se.CSRFToken == "" {
		writeAPIError(w, http.StatusUnauthorized, "未登录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": se.CSRFToken})
}
