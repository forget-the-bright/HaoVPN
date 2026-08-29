// Package clientapp CLI/GUI 共用 VPN 拨号引擎。
//
// 关键文件：
//   engine_state.go — State、Engine 结构、NewEngine、状态查询
//   engine_lifecycle.go — Start/Stop、杀开关 protectThenClearRoutes
//   engine_connect.go — onConnect 握手、TUN readLoop
//   runtime.go — TUN/路由/DNS applyPolicy
//   credentials.go — ResolveCredentials（YAML 用户名可配服务库密码）、PromptPassword
//   fatal_auth.go — IsFatalHandshakeError（优先 auth/sessionmgr 哨兵 errors.Is）
//   bootstrap.go — CLI RunCLI/StopCLI
//   service_windows.go — Windows 服务
//
// 上游：cmd/client、cmd/client-gui、internal/clientgui。
// 下游：transport、tunnel、netstack、security、config、auth（致命错误哨兵）。
// 并发：Engine 持 mu/activeMu；transport 与 tunReadLoop 各 goroutine。
// 不变量：断线先 EnableKillSwitch 再清路由；策略以握手应答为准；
// 锁定/错密等致命鉴权错误停止自动重连（文案与 auth.ErrLoginLocked 对齐）。
package clientapp
