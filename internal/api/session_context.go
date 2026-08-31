package api

import (
	"context"
	"net/http"

	"haovpn/internal/auth"
)

// ctxKeySession 将已校验的 Web 会话存入 request.Context，避免 handler 重复解析 Cookie。
type ctxKeySession struct{}

// withRequestSession 克隆请求并注入会话（requireAuth / requireAuthPage 成功后调用）。
func withRequestSession(r *http.Request, se auth.SessionEntry) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKeySession{}, se))
}

// sessionFromContext 读取 middleware 注入的会话；未注入时 ok=false。
func sessionFromContext(r *http.Request) (auth.SessionEntry, bool) {
	v := r.Context().Value(ctxKeySession{})
	if v == nil {
		return auth.SessionEntry{}, false
	}
	se, ok := v.(auth.SessionEntry)
	return se, ok
}
