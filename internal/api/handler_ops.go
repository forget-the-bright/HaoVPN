package api

import (
	"net/http"
	"strings"

	"haovpn/internal/audit"
	"haovpn/internal/health"
	"haovpn/internal/logstore"
	"haovpn/internal/logger"
	"haovpn/internal/paginate"
	"haovpn/internal/persist"
	"haovpn/internal/readmodel"
	"haovpn/internal/version"
)

// handleHealth 健康检查（GET /api/v1/health，公开）。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbOK, online, recent := s.dataplaneSnapshot()
	st := health.NewStatus(s.startedAt, online, dbOK, s.tunOK, s.natOK, recent)
	writeJSON(w, http.StatusOK, st)
}

// handleSystemInfo 返回构建版本信息（GET /api/v1/system/info，公开）。
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.Info())
}

// handleAudit 分页查询管理审计日志（GET /api/v1/audit）。
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paginate.ParseLimitOffset(q, 50, 500)
	logs, total, err := s.store.ListAuditLogsFiltered(readmodel.AuditListFilter{
		Action: q.Get("action"),
		Since:  parseSinceQuery(r),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writePage(w, http.StatusOK, persist.AuditEntriesToViews(logs), total, limit, offset)
}

// handleDashboard 仪表盘摘要 JSON（GET /api/v1/dashboard）。
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	dbOK, online, recent := s.dataplaneSnapshot()
	writeJSON(w, http.StatusOK, health.DashboardMap(
		s.startedAt, online, dbOK, s.tunOK, s.natOK, recent,
	))
}

// dataplaneSnapshot 聚合 DB/在线数/近期错误，供 health 与 dashboard 共用。
func (s *Server) dataplaneSnapshot() (dbOK bool, online int, recent []string) {
	dbOK = s.store.DB().Ping() == nil
	online = s.sessions.OnlineCount()
	recent = logger.RecentErrors()
	return dbOK, online, recent
}

// handleLogs 读取实时或历史日志（GET /api/v1/logs）。
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	q := r.URL.Query()
	source := stringsToLowerTrim(q.Get("source"))
	if source == "" {
		source = "live"
	}
	tail := parseLogTailQuery(q.Get("tail"))

	switch source {
	case "history":
		if s.logStore == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"source": "history", "items": []any{}, "total": 0, "lines": []string{},
			})
			return
		}
		limit := tail
		offset := paginate.ParseIntDefault(q.Get("offset"), 0)
		items, total, err := s.logStore.Query(logstore.Query{
			Level: q.Get("level"), Keyword: q.Get("q"), Since: parseSinceQuery(r),
			Limit: limit, Offset: offset,
		})
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var lines []string
		for _, it := range items {
			lines = append(lines, it.Line)
		}
		lines = redactLogLines(lines)
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
			writeAPIError(w, http.StatusInternalServerError, err.Error())
			return
		}
		lines = redactLogLines(lines)
		writeJSON(w, http.StatusOK, map[string]any{
			"source": source, "lines": lines, "truncated": truncated, "file": path,
		})
	}
}

// handleBackup 下载 SQLite 主库备份（GET /api/v1/backup）。
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	se, _ := s.sessionFromRequest(r)
	s.audit.Log(&se.UserID, "db_backup", "system", nil, s.clientIP(r), nil)
	http.ServeFile(w, r, s.cfg.Database.Path)
}

// LogPublicBindAudit 公网绑定启动时写审计记录。
func LogPublicBindAudit(auditLog *audit.Logger) {
	auditLog.Log(nil, "management_public_bind_enabled", "system", nil, "", map[string]string{
		"message": "用户已开启 allow_public_bind，管理口暴露风险自担",
	})
}

func stringsToLowerTrim(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
