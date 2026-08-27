// Package readmodel 定义 Web/API 读模型 DTO，与 SQLite 存储层解耦。
//
// 关键文件：types.go — UserListItem、MonitorAccountRow、各 ListFilter。
//
// 上游：api 序列化 JSON；persist 查询 scan 填充。
// 下游：无 internal 依赖。
// 不变量：JSON tag 与 WebUI 契约一致；SQL 留在 persist。
package readmodel
