// Package safeutil 提供安全 goroutine 包装、优雅关闭与日志限频辅助
//（GoSafe、Shutdown、Ticker、RetryN、ExpBackoff、PollUntil、IsCanceled/IsDeadline/Check、AllowEvery）。
//
// 关键文件：
//   goroutine.go — GoSafe
//   shutdown.go / ticker.go — 优雅关闭与可停 Ticker
//   retry.go — RetryN、ExpBackoff
//   poll.go — PollUntil
//   context.go — IsCanceled、IsDeadline、Check
//   throttle.go — AllowEvery（日志/埋点/session_stats CAS 限频；非业务配额）
//
// 上游：serverapp（retention/TUN 读/API listen）、clientapp（tunReadLoop、applyPolicy abort、DNS settle、quiesce 日志）、
// netstack（Setup abort 与 forward_only 区分）、transport（Dial/AcceptConn、ReconnectClient、queue full WARN）、
// sessionmgr（异步关旧连接、伪造源/越权 WARN 限频、stats 刷新）、logstore（writerLoop）、singleinstance（acceptLoop）、
// clientgui（登录超时 IsDeadline、autostart 可中止轮询）。
// 下游：标准库 context/sync/atomic；panic 时写 logger 不崩溃进程。
// 并发：GoSafe 每调用启动新 goroutine；Shutdown.Wait 阻塞至子任务结束或超时；AllowEvery 对同一 atomic 安全。
// 不变量：生产路径长生命周期 goroutine 须 GoSafe（裸 go 禁止）；panic 必须 recover 并记录；
// Shutdown 先 cancel 再 Wait；重连退避用 ExpBackoff（与 transport 测试对齐）；
// 可中止 deadline 轮询用 PollUntil（勿再手写 for+Sleep）；
// context 取消判定统一 IsCanceled/Check；仅截止用 IsDeadline（GUI 超时文案）；禁止 Error() 字符串比对；
// 日志刷屏与同类节流统一 AllowEvery，禁止各包再手写 Load/CAS 间隔。
package safeutil
