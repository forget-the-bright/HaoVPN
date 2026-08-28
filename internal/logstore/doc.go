// Package logstore 提供结构化运行日志独立 SQLite，供 WebUI 分页检索与保留策略清理。
//
// 时间列 layout 统一用 timeutil（与 persist 一致，避免硬编码或反向依赖 persist）。
// 上游：serverapp 可选启用；api 查询；maintenance 保留清理。
// 下游：modernc.org/sqlite、paginate、timeutil、logger。
package logstore
