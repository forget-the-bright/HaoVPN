// Package audit records management operations.
package audit

import (
	"encoding/json"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// Logger writes audit entries to SQLite.
type Logger struct {
	store *persist.Store
}

// New creates an audit logger.
func New(store *persist.Store) *Logger {
	return &Logger{store: store}
}

// Log records an audit event.
func (l *Logger) Log(actorUserID *int64, action, targetType string, targetID *int64, clientIP string, detail any) {
	var detailJSON string
	if detail != nil {
		b, _ := json.Marshal(detail)
		detailJSON = string(b)
	}
	entry := persist.AuditEntry{
		ActorUserID: actorUserID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		ClientIP:    clientIP,
		DetailJSON:  detailJSON,
	}
	if err := l.store.InsertAuditLog(entry); err != nil {
		logger.Error("audit log write failed: %v", err)
	}
}
