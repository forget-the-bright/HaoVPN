// Package clientapp 提供 CLI/GUI 共用的 VPN 拨号引擎。
//
// 关键文件（第十六轮同包拆分）：
//   engine_*.go — 状态机、Start/Stop、握手连接、protectForReconnect
//   runtime.go — runtime 结构、allowedIPs、close、write
//   runtime_policy.go — applyPolicy
//   runtime_routes.go — 路由安装/差分/清理
//   runtime_tun.go — TUN 读循环与上送过滤
//   policy_diff.go / via_exit.go — 策略差分、via/ICS 指纹
//   credentials.go / fatal_auth.go / dial_errors.go — 凭据、致命鉴权（委托 autherr）、封禁友好提示
//   route_view.go — ManagedRouteView（GUI 托盘 DTO）
//   service_windows.go / service_other.go — SCM 薄封装（写路径在 autostart）
//
// 上游：cmd/client、clientgui。
// 下游：transport、tunnel、netstack、config、auth、netutil、autostart。
// 并发：Engine 持锁；临时断线可保留数据面；Stop 全清。
// 不变量：致命鉴权错误停止自动重连；策略以握手应答为准。
package clientapp
