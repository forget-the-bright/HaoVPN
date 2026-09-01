// Package winnet 封装 Windows 网卡/LUID/IP Helper/netsh/PowerShell 公共能力。
//
// 职责边界（叶子包；netstack/tun/clientapp 调用，勿反向依赖业务）：
//   - 解析 Wintun 配置名 → 系统 ifIndex / netsh 别名（InterfaceIndex / FindAdapterIfIndex）
//   - 读/写地址与路由、DNS（IP Helper 优先，失败回退 netsh）
//   - PowerShell：RunPSOneShot / RunPSOneShotContext / RunPSBestEffort / RunPSBestEffortContext（一律一进程一脚本；取消 Kill）
//   - PS 模板单一真相源：ps_snippets.go（找网卡、ICS 关共享、WinNAT New/Remove、孤儿 Wintun 清理）
//   - SKU：IsWindowsHomeSKU（家庭版跳过 WinNAT）
//   - ICS 残留探测与清理；RememberICSPair + DisableICSPair(Context) / DisableICSSessionContext（退出快关）
//   - 禁止再引入常驻 PowerShell 主机（历史 Shutdown 空钩已删除）
//
// 关键文件：options.go、sku*.go、iphlp_*、dns_*、ps_windows.go、ps_snippets.go、ps_assemble_windows.go、
// address_windows.go、egress_*.go、prefer_vpn_light_windows.go、default_route_*.go、ics_*、dns_restore_*.go、orphan*.go、resolver_*。
//
// 关联：netstack 只编排「何时」EnableSharing/装路由；脚本片段须来自本包，禁止业务包复制 Get-NetAdapter。
package winnet
