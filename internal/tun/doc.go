// Package tun 提供跨平台 TUN 设备抽象（Linux / Windows Wintun / macOS utun）。
//
// 关键文件：
//   tun.go — Device 接口、Open、parseCIDR（包内；外部用 netutil.ParseCIDR）
//   tun_windows.go — Wintun 会话（复用、配 IP、Read/Write）
//   wintun_adapter_windows.go — 适配器 Open/Create、固定 GUID；孤儿清理调 winnet.BuildPrepareWintunOrphanScript
//   wintun_log_windows.go — Wintun DLL 日志桥接（预期噪声降为 Debug）
//   warmup_*.go — GUI 预热适配器句柄（Close 会话不卸适配器）
//   wintundll/ — Windows DLL embed 与 Ensure
//
// 上游：clientapp/runtime、serverapp。
// 下游：winnet（Windows IP/索引、孤儿 PS）、brand（默认网卡名）。
// 并发：Device Read/Write 由调用方串行或各自 goroutine；winDevice 内部 mu。
// 不变量：Close 保留 Wintun 适配器供下次快速 Open；CIDR 须 IPv4；孤儿脚本不在本包维护第二套。
package tun
