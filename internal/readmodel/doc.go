// Package readmodel 定义 Web/API 读模型 DTO，与 SQLite 存储层解耦。
//
// 关键文件：types.go（UserList*）、monitor.go、audit.go、peers.go（PeerRoute/Access/LANRegistry 视图）。
// 上游：api 序列化 JSON；persist JOIN 查询填充行结构。
// 下游：timeutil（RFC3339）；不 import persist 存储类型（转换在 api/persist）。
// 并发：纯数据结构与转换函数，无状态。
// 不变量：JSON 字段名与 WebUI 契约一致；SQL 逻辑留在 persist。
package readmodel
