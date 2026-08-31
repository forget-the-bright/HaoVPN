// Package safeutil 提供安全 goroutine 包装与优雅关闭辅助（GoSafe、Shutdown、Ticker、RetryN、ExpBackoff）。
//
// 上游：serverapp（retention/TUN 读/API listen）、clientapp（tunReadLoop）、
// transport（Dial/AcceptConn 读写心跳、ListenTLS accept/handle、ReconnectClient）、
// sessionmgr（异步关旧连接）、logstore（writerLoop）、singleinstance（acceptLoop）。
// 下游：标准库 context/sync；panic 时写 logger 不崩溃进程。
// 并发：GoSafe 每调用启动新 goroutine；Shutdown.Wait 阻塞至子任务结束或超时。
// 不变量：生产路径长生命周期 goroutine 须 GoSafe（裸 go 禁止）；panic 必须 recover 并记录；
// Shutdown 先 cancel 再 Wait；重连退避用 ExpBackoff（与 transport 测试对齐）。
package safeutil
