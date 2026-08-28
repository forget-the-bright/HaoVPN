// Package health 提供服务端启动自检与运行态健康/Dashboard 读模型。
//
// 关键文件：health.go（Checker、NewStatus）、dashboard.go（DashboardMap）。
// 上游：serverapp 启动 RunStartupChecks；api handleHealth/handleDashboard。
// 下游：config、persist、fileutil、logger。
// 并发：Checker/Status 只读；无 goroutine。
// 不变量：自检失败 Fatal 拒绝启动（证书/DB）；Dashboard 与 health 字段对齐。
package health
