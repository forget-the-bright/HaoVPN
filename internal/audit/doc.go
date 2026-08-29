// Package audit 封装管理操作审计写入（audit_logs 表）与展示标签。
//
// 上游：api 写操作（登录、开户、导出、改策略等）。
// 下游：persist.Store InsertAudit。
// 关键文件：audit.go（写入）、labels.go（动作/目标中文，与 hardening 对照表同源）。
// 并发：多 HTTP goroutine 并发 Log；依赖 SQLite 串行写。
// 不变量：敏感字段不得出现在 DetailJSON；ActorUserID 可为 nil（未登录失败）；库表存英文码，展示层用 labels。
package audit
