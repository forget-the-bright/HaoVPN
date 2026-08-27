//go:build windows

// Package wintundll 确保 wintun.dll 存在于进程目录或嵌入释放。
//
// 上游：tun.openPlatform（Windows Open 前调用 Ensure）。
// 下游：无 internal 依赖；可能写 exe 同目录 DLL。
// 不变量：Ensure 幂等；失败时 tun.Open 返回明确错误。
package wintundll
