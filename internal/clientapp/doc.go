// Package clientapp 提供 CLI/GUI 共用的 VPN 拨号引擎。
//
// 文件簇（按职责软边界，不拆子包以免 import 环）：
//   engine_lifecycle.go / engine_state.go / engine_connect.go — 状态机、Start/Stop、握手连接、protectForReconnect
//   hard_restart.go — HardRestart / WaitDNSReady（手动全量重连；与 Soft 重连双路径）
//   dial_errors.go / fatal_auth.go — 拨号 UX 文案与致命判定（直接 autherr+dialerr；禁止薄 Is* re-export）
//   runtime.go / runtime_policy.go / runtime_routes.go / runtime_tun.go — TUN/路由/策略数据面
//   policy_diff.go / via_exit.go — 策略差分、via/ICS（空 local_lans 先 HasICSResidue 再清理）
//   credentials.go / bootstrap.go — 凭据与启动编排（含 SaveServiceCredentials 门面）
//   warmup.go — WarmupTun（GUI 预热经此，禁止 clientgui→tun）
//   route_view.go — ManagedRouteView（GUI 托盘 DTO，避免 clientgui→tunnel）
//   service_windows.go / service_other.go — SCM 薄封装（写路径在 autostart）
//
// 上游：cmd/client、clientgui。
// 下游：transport、tunnel、dialerr、autherr、netstack、config、auth、netutil、autostart、tun、credentials。
// 禁止：直接 import winnet（经 netstack 门面 ConfigureWindows / HasICSResidue 等）。
// 并发：Engine 持锁；临时断线可保留数据面；Stop 全清。
// 不变量：
//   - 致命鉴权/拨号错误停止自动重连；策略以握手应答为准；reportFirstFailure 保留 errors.Is 哨兵。
//   - Soft 重连：transport.ReconnectClient + protectForReconnect（保 dataplane）。
//   - Hard 重连：HardRestart（全清后再拨）；GUI 禁止第三套编排。
package clientapp
