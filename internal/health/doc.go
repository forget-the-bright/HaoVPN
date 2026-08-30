// Package health 提供服务端启动自检与运行态健康/Dashboard 读模型。
//
// 关键文件：health.go（Checker、NewStatus）、dashboard.go（DashboardMap）。
// 上游：serverapp 启动 RunStartupChecks；api handleHealth / handleDashboard。
// 下游：config、persist、fileutil、logger。
//
// 公开 /api/v1/health：api 层只返回 ok + uptime_sec（不序列化完整 Status），
// 避免未登录探测 db_ok/tun_ok/在线数；完整字段仅 Dashboard（需登录）。
package health
