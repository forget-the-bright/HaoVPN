// Package netstack 管理 TUN 侧路由、DNS 与杀开关（按平台分文件实现）。
//
// 关键文件（Windows）：
//   forward_windows.go — IP 转发（注册表 + Set-NetIPInterface）
//   nat_windows.go — WinNAT；家庭版 sku_home 直进 ICS；Get-NetNat/New-NetNat 仅 RunPSOneShot
//   ics_lifecycle.go — ICSDisable / ICSPreserve（拆除时是否关共享）
//   ics_egress_windows.go — ICS 出站网卡解析（IP Helper / PS）
//   ics_enable_windows.go — 有 137→reuse_live；无→Restart+Enable→Go iphlp Prefer
//   ics_plan.go — 多 local_lans 出站网卡规划（纯函数，无 COM）
//   nat_outcome.go — Setup 结果（ActiveCIDRs / SkipCIDRs / ICS hint）
//   route_ops_windows.go — 客户端 on-link 路由 ADD/DELETE
//   dns_windows.go — Apply/Restore DNS（经 winnet netsh）
//   killswitch_windows.go — Enable/Disable/Remove 公开 API + 引擎
//   killswitch_wfp_filter_windows.go — Block 过滤器安装/删除
//   killswitch_wfp_enum_windows.go — 按层枚举产品子层
//   common.go — WFPFilterRef / SelectProductFilterIDs（跨平台）
//   winnet_facade.go — ConfigureWindows / HasICSResidue / CleanupICSResidue(Context) /
//                      RemoveICSAddressesKeepVPN / ScrubTUNDefaultRouteFast /
//                      PreferVPNAfterSoftIPReplace / ReplaceTUNIPv4KeepICS
//
// 上游：clientapp runtime、serverapp TUN/NAT Setup。
// 下游：winnet（Windows 网卡/PS/SKU）、netutil（CIDR）、platform（无窗口子进程）、safeutil（IsCanceled/Check）。
// 并发：Setup(ctx)/Teardown(ctx) 由调用方串行；路由变更非线程安全须单 goroutine 编排。
// 不变量：不 import tun；断线杀开关先于清路由（clientapp 编排）；
// PowerShell：NetNat/ICS 用 RunPSOneShot(Context)；清理用 BestEffort(Context)；禁止 raw platform.Command。
// Setup(ctx) 取消不得 forward_only 吞成功；Teardown 正常路径传 Background 以免跳过 ICS 清理。
// ICS 清理不经 winnet_facade 的 DisableICS* 导出；Teardown → ics_enable_windows.disableICSPlatform → winnet。
package netstack
