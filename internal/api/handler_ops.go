package api

import (
	"encoding/json"
	"net/http"
	"strconv"
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
//
// 故意不返回 recent_errors：公开探针不应泄漏 WARN/ERROR 栈与路径；
// 近期错误仅经需登录的 /api/v1/dashboard 暴露。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbOK, online, _ := s.dataplaneSnapshot()
	st := health.NewStatus(s.startedAt, online, dbOK, s.tunOK, s.natOK, nil)
	writeJSON(w, http.StatusOK, st)
}

// handleSystemInfo 返回构建版本信息（GET /api/v1/system/info，公开）。
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.Info())
}

// handleAudit 分页查询管理审计日志（GET /api/v1/audit）。
//
// 返回项含 action_zh / target_type_zh；target_type=user 时尽量填充 target_username。
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
		writeInternalError(w, err)
		return
	}
	views := persist.AuditEntriesToViews(logs)
	enrichAuditViews(s.store, views)
	writePage(w, http.StatusOK, views, total, limit, offset)
}

// enrichAuditViews 填充审计展示字段：中文标签与用户名（不改库表英文码）。
func enrichAuditViews(store *persist.Store, views []readmodel.AuditLogView) {
	for i := range views {
		v := &views[i]
		v.ActionZH = audit.ActionLabel(v.Action)
		v.TargetTypeZH = audit.TargetTypeLabel(v.TargetType)
		if v.TargetType != "user" || v.TargetID == nil || store == nil {
			continue
		}
		name := store.UsernameByID(*v.TargetID)
		// UsernameByID 不存在时返回 "#id" 占位；再尝试 detail_json.username（已删账号）
		placeholder := "#" + strconv.FormatInt(*v.TargetID, 10)
		if name != "" && name != placeholder {
			v.TargetUsername = name
			continue
		}
		if u := usernameFromDetailJSON(v.DetailJSON); u != "" {
			v.TargetUsername = u
		}
	}
}

// usernameFromDetailJSON 从审计 detail_json 提取 username 字段（开户/重置等会写入）。
func usernameFromDetailJSON(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(detail), &m); err != nil {
		return ""
	}
	return strings.TrimSpace(m["username"])
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
	if !requireMethod(w, r, http.MethodGet) {
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
			writeInternalError(w, err)
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
			writeInternalError(w, err)
			return
		}
		lines = redactLogLines(lines)
		writeJSON(w, http.StatusOK, map[string]any{
			"source": source, "lines": lines, "truncated": truncated, "file": path,
		})
	}
}

// handleBackup 下载 SQLite 主库备份（POST /api/v1/backup；须 CSRF，防 SameSite=Lax 跨站 GET 拖库）。
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
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

