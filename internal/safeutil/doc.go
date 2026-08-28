// Package safeutil 提供安全 goroutine 包装与优雅关闭辅助（GoSafe、Shutdown、Ticker）。
//
// 上游：serverapp（retention/TUN 读/API listen）、clientapp（tunReadLoop）。
// 下游：标准库 context/sync；panic 时写 logger 不崩溃进程。
// 并发：GoSafe 每调用启动新 goroutine；Shutdown.Wait 阻塞至子任务结束或超时。
// 不变量：GoSafe 内 panic 必须 recover 并记录；Shutdown 先 cancel 再 Wait。
package safeutil
