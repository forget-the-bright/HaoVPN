// Package tun 提供跨平台 TUN 设备抽象（Linux / Windows Wintun / macOS utun）。
//
// 关键文件：
//   tun.go — Device 接口、Open、ParseCIDR
//   tun_windows.go — Wintun 会话（复用、配 IP、Read/Write）
//   wintun_adapter_windows.go — 适配器 Open/Create、孤儿网卡清理、固定 GUID
//   wintun_log_windows.go — Wintun DLL 日志桥接（预期噪声降为 Debug）
//   wintundll/ — Windows DLL embed 与 Ensure
//
// 上游：clientapp/runtime、serverapp。
// 下游：winnet（Windows IP/索引）、brand（默认网卡名）。
// 并发：Device Read/Write 由调用方串行或各自 goroutine；winDevice 内部 mu。
// 不变量：Close 保留 Wintun 适配器供下次快速 Open；CIDR 须 IPv4。
package tun
