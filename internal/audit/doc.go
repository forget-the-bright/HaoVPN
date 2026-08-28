// Package audit 封装管理操作审计写入（audit_logs 表）。
//
// 上游：api 写操作（登录、开户、导出、改策略等）。
// 下游：persist.Store InsertAudit。
// 并发：多 HTTP goroutine 并发 Log；依赖 SQLite 串行写。
// 不变量：敏感字段不得出现在 DetailJSON；ActorUserID 可为 nil（未登录失败）。
package audit
