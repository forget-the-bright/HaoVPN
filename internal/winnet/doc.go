// Package winnet 封装 Windows 网卡/LUID/IP Helper/netsh/PowerShell 公共能力。
//
// 职责边界（叶子包；netstack/tun/clientapp 调用，勿反向依赖业务）：
//   - 解析 Wintun 配置名 → 系统 ifIndex / netsh 别名（InterfaceIndex / FindAdapterIfIndex）
//   - 读/写地址与路由、DNS（IP Helper 优先，失败回退 netsh）
//   - PowerShell：RunPSOneShot / RunPSOneShotContext / RunPSBestEffort*（一律一进程一脚本；取消 Kill）
//   - PS 模板：ps_snippets.go；ICS stdout 解析：ps_log.go（LogICSPowerShellLines）
//   - ICS 137 探测：HasICSResidue（按名）/ InterfaceHasICSPrivate（按 ifIndex）
//   - PreferVPN 主路径：prefer_vpn_light_windows.go（iphlp）；PS 仅回退
//   - SKU：IsWindowsHomeSKU；ICS 会话：RememberICSPair + DisableICSSessionContext
//
// 关键文件：options.go、sku*.go、iphlp_*、ics_probe.go、ps_log.go、ps_snippets.go、
// prefer_vpn_light_windows.go、default_route_*.go、ics_*、egress_*.go、orphan*.go。
//
// 关联：netstack 只编排「何时」EnableSharing/装路由；脚本片段须来自本包，禁止业务包复制 Get-NetAdapter。
package winnet
