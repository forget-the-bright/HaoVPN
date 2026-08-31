package api

import (
	"net/http"

	"haovpn/internal/paginate"
	"haovpn/internal/persist"
)

// handleSecurityEvents 分页查询探针安全事件（GET /api/v1/security/events）。
func (s *Server) handleSecurityEvents(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	q := r.URL.Query()
	limit, offset := paginate.ParseLimitOffset(q, 50, 500)
	items, total, err := s.store.ListSecurityEvents(persist.SecurityEventFilter{
		ClientIP:  q.Get("ip"),
		Signature: q.Get("signature"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writePage(w, http.StatusOK, toEventViews(items), total, limit, offset)
}
