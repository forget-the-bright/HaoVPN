// Package netstack 管理 TUN 侧路由、DNS 与杀开关（按平台分文件实现）。
//
// 关键文件（Windows）：
//   forward_windows.go — IP 转发（注册表 + Set-NetIPInterface）
//   nat_windows.go — WinNAT New-NetNat / teardown
//   ics_nat_windows.go — ICS 回退 SNAT、出站网卡发现、disableICS
//   route_ops_windows.go — 客户端 on-link 路由 ADD/DELETE
//   dns_windows.go — Apply/Restore DNS（经 winnet netsh）
//   killswitch_windows.go — Enable/Disable/Remove 公开 API + 引擎
//   killswitch_wfp_filter_windows.go — Block 过滤器安装/删除
//   killswitch_wfp_enum_windows.go — 按层枚举产品子层
//   common.go — WFPFilterRef / SelectProductFilterIDs（跨平台）
//
// 上游：clientapp runtime、serverapp TUN/NAT Setup。
// 下游：winnet（Windows 网卡/PS）、netutil（CIDR）、platform（无窗口子进程）。
// 并发：Setup/Teardown 由调用方串行；路由变更非线程安全须单 goroutine 编排。
// 不变量：不 import tun；断线杀开关先于清路由（clientapp 编排）；
// PowerShell 禁止 raw platform.Command，一律 winnet.RunPS / RunPSBestEffort。
package netstack
