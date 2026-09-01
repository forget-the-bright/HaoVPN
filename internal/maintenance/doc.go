// Package maintenance 提供服务端后台维护任务（数据保留清理等），与 HTTP api 解耦。
//
// 关键文件：retention.go — RunDataRetention / StartRetentionLoop（审计、日志、封禁 prune）。
//
// 上游：serverapp 在启动后调用 StartRetentionLoop。
// 下游：persist、logstore、config、safeutil、logger。
// 并发：StartRetentionLoop 经 safeutil.GoSafe 启动独立 goroutine 跑 ticker；RunDataRetention 可被并发调用但通常单线程。
// 不变量：cfg 为 nil 或 store 为 nil 时 RunDataRetention 直接返回，不 panic；goroutine panic 由 GoSafe 恢复。
// 关联：保留天数默认值见 config 服务端 Database 段；清理 SQL 在 persist/logstore。
package maintenance
