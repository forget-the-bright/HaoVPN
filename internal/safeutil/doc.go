// Package safeutil 提供 panic 安全协程包装（GoSafe）与优雅关闭（Shutdown/信号处理）。
//
// serverapp 与 clientapp 用 Shutdown 管理 TUN 读循环等后台任务生命周期。
package safeutil
