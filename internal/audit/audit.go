package audit

import (
	"encoding/json"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// Logger 将管理操作审计事件写入 SQLite（经 persist.Store）。
//
// 字段：
//   store — 共享持久化层；InsertAuditLog 失败时仅打 Error，不向上抛错。
//
// 线程安全：Log 无内部锁；依赖 store 的并发语义；适合 HTTP handler 直接调用。
type Logger struct {
	store *persist.Store
}

// New 构造审计记录器。
//
// 参数：store 非空且已打开数据库。
// 返回：绑定同一 store 的 *Logger。
// 副作用：无。
func New(store *persist.Store) *Logger {
	return &Logger{store: store}
}

// Log 记录一条结构化审计事件并尽力写入数据库。
//
// 参数：actorUserID 可为 nil（系统动作）；action/targetType 为业务动词与对象类型；
// targetID 可为 nil；clientIP 为操作来源 IP；detail 任意 JSON 可序列化结构，nil 则 DetailJSON 为空。
// 返回：无；InsertAuditLog 失败时打 Error 日志，不中断调用方。
// 副作用：写 audit 表一行。
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
