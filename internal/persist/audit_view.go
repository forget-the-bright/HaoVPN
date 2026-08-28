package persist

import "haovpn/internal/readmodel"

// AuditEntriesToViews 将审计存储行批量转为 API 读模型。
func AuditEntriesToViews(entries []AuditEntry) []readmodel.AuditLogView {
	out := make([]readmodel.AuditLogView, len(entries))
	for i, e := range entries {
		out[i] = readmodel.AuditLogViewFrom(
			e.ID, e.ActorUserID, e.Action, e.TargetType, e.TargetID,
			e.ClientIP, e.DetailJSON, e.CreatedAt,
		)
	}
	return out
}
