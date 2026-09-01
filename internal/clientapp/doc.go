// Package clientapp 提供 CLI/GUI 共用的 VPN 拨号引擎。
//
// 文件簇（按职责软边界，不拆子包以免 import 环）：
//   engine_lifecycle.go / engine_state.go / engine_connect.go — 状态机、Start/Stop、握手连接、protectForReconnect
//   engine_bootstrap.go — PrepareEngine / StartAndWaitFirstAuth / DefaultGUIRunOptions（GUI 与 CLI 启动契约）
//   hard_restart.go — HardRestart / waitDNSReadyAbort（手动全量重连；abort 可中止 DNS settle）
//   dial_errors.go / fatal_auth.go / connect_failure.go / connect_warn.go — 拨号/首连/已连告警 UX 文案
//   runtime.go / runtime_policy.go / runtime_routes.go / runtime_tun.go — TUN/路由/策略数据面
//   policy_diff.go / via_exit.go — 策略差分、via/ICS（空 local_lans 先 HasICSResidue 再清理）
//   credentials.go / bootstrap.go — 凭据与 RunOptions / RunCLI / RunServiceLoop
//   autostart_facade.go — 登录/服务自启与 ServiceStopAndWait（GUI 禁 direct autostart）
//   single_instance_hint.go — CLI/GUI 单实例冲突文案
//   hooks.go — AttachDataplaneHook
//   warmup.go — StartWarmupAsync（RunOptions.WarmupTun 触发内部 warmupTun）
//   route_view.go — ManagedRouteView（GUI 托盘 DTO，避免 clientgui→tunnel）
//   service_windows.go / service_other.go — SCM 薄封装（写路径在 autostart）
//
// 上游：cmd/client、clientgui。
// 下游：transport、tunnel、dialerr、autherr、netstack、config、auth、netutil、autostart、tun、credentials。
// 禁止：直接 import winnet（经 netstack 门面 ConfigureWindows / HasICSResidue 等）。
// 并发：Engine 持锁；临时断线可保留数据面；Stop 先 cancel runCtx（打断 applyPolicy/via）再全清。
// 不变量：
//   - 致命鉴权/拨号错误停止自动重连；策略以握手应答为准。
//   - Soft 重连：transport.ReconnectClient + protectForReconnect（保 dataplane）。
//   - Hard 重连：HardRestart（全清后再拨）；GUI 禁止第三套编排。
//   - 启动：交互 CLI 用 RunCLI；GUI 登录用 PrepareEngine；服务用 RunServiceLoop。
package clientapp
