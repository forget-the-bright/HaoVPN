// Package platform 封装跨平台提权（Windows UAC RelaunchElevated）与无控制台子进程（route/netsh）。
//
// Windows 实现见 cmd_windows.go / elevate_windows.go；非 Windows 为 honest stub。
package platform
