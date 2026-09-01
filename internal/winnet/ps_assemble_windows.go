//go:build windows

package winnet

import "fmt"

// PSAssignAdapterAndPreferVPN 组装 AssignAdapterIf + PreferVPNAfterICS（standalone 回退路径）。
//
// tunIfIndex 为 0 时 PreferVPN 片段内用 $prvIdx（由 AssignAdapter 赋值）。
func PSAssignAdapterAndPreferVPN(configName, vpnIP string, tunIfIndex int) string {
	return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
%s
if (-not $if) { throw '未找到 Wintun 网卡' }
$prvIdx = $if.ifIndex
%s
`, PSSnippetAssignAdapterIf(configName), PSSnippetPreferVPNAfterICS(vpnIP, tunIfIndex))
}

// PSAssignAdapterAndSkipAsSourceOnly 软换 VPN IP 后的轻量 PS（已知 ifIndex，仅 SkipAsSource）。
func PSAssignAdapterAndSkipAsSourceOnly(vpnIP string, tunIfIndex int) string {
	return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$prvIdx = %d
%s
`, tunIfIndex, PSSnippetSkipAsSourceOnly(vpnIP, tunIfIndex))
}
