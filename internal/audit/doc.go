// Package audit 记录 Web 管理操作审计日志（写入 persist SQLite）。
//
// 由 api handler 在敏感操作后调用；保留策略见 config.Database.AuditRetentionDays。
package audit
