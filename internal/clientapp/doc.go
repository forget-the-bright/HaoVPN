// Package clientapp 提供 CLI/GUI 共用的 VPN 拨号引擎。
//
// 文件簇（按职责软边界，不拆子包以免 import 环）：
//   engine_lifecycle.go / engine_state.go / engine_connect.go — 状态机、Start/Stop、握手连接、protectForReconnect
//   dial_errors.go / fatal_auth.go — 拨号 UX 文案与致命判定（直接 autherr+dialerr；禁止薄 Is* re-export）
//   runtime.go / runtime_policy.go / runtime_routes.go / runtime_tun.go — TUN/路由/策略数据面
//   policy_diff.go / via_exit.go — 策略差分、via/ICS（空 local_lans 先 HasICSResidue 再清理）
//   credentials.go / bootstrap.go — 凭据与启动编排
//   route_view.go — ManagedRouteView（GUI 托盘 DTO，避免 clientgui→tunnel）
//   service_windows.go / service_other.go — SCM 薄封装（写路径在 autostart）
//
// 上游：cmd/client、clientgui。
// 下游：transport、tunnel、dialerr、autherr、netstack、config、auth、netutil、autostart。
// 并发：Engine 持锁；临时断线可保留数据面；Stop 全清。
// 不变量：致命鉴权/拨号错误停止自动重连；策略以握手应答为准；reportFirstFailure 保留 errors.Is 哨兵。
package clientapp
