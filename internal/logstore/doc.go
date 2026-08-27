// Package logstore 提供结构化运行日志独立 SQLite，供 WebUI 分页检索与保留策略清理。
//
// 由 serverapp 可选启用；api 提供查询接口，StartRetentionLoop 定期清理。
package logstore
