// Package winnet 封装 Windows 网卡/LUID/IP Helper/netsh/PowerShell 公共能力。
//
// 职责边界（叶子包；netstack/tun/clientapp 调用，勿反向依赖业务）：
//   - 解析 Wintun 配置名 → 系统 ifIndex / netsh 别名（InterfaceIndex / FindAdapterIfIndex）
//   - 读/写地址与路由、DNS（IP Helper 优先，失败回退 netsh）
//   - PowerShell：RunPS / RunPSOneShot / RunPSBestEffort（一律一进程一脚本）
//   - PS 模板单一真相源：ps_snippets.go（找网卡、ICS 关共享、孤儿 Wintun 清理）
//   - SKU：IsWindowsHomeSKU（家庭版跳过 WinNAT）
//   - ICS 残留探测与清理；RememberICSPair + DisableICSPair（退出快关）
//   - Shutdown：进程退出挂点（当前空操作；曾用于常驻 PS，已删除）
//
// 关键文件：options.go、sku*.go、iphlp_*、dns_*、ps_windows.go、ps_snippets.go、
// address_windows.go、ics_*、resolver_*。
//
// 关联：netstack 只编排「何时」EnableSharing/装路由；脚本片段须来自本包，禁止业务包复制 Get-NetAdapter。
package winnet
