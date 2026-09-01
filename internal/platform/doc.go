// Package platform 封装跨平台提权检测与无窗口子进程（route/netsh 等）。
//
// 关键文件：elevate_*.go — UAC/RelaunchElevated；cmd_*.go — 无窗口 exec；cmderr.go — CommandOutputError。
//
// 上游：netstack、winnet、cmd/client-gui（UAC）、clientgui（IsAdmin 检查）。
// 下游：os/exec；Windows 使用 CREATE_NO_WINDOW。
// 并发：Command 每次新建子进程；无共享可变状态。
// 不变量：子进程 stderr 经 CommandOutputError 包装为中文可读错误；非 Windows 为 stub。
package platform
