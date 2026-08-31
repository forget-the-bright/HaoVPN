//go:build windows

package winnet

import (
	"golang.org/x/sys/windows/registry"

	"haovpn/internal/logger"
)

// IsWindowsHomeSKU 读 HKLM CurrentVersion，判断家庭版/Core（无 PS）。
//
// 用途：via/NAT 启动前跳过无意义的 Get-NetNat/New-NetNat，直接 ICS。
// 读失败时返回 false（保守：仍尝试 WinNAT，失败再回退）。
func IsWindowsHomeSKU() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		logger.Debug("sku_home registry open fail: %v", err)
		return false
	}
	defer k.Close()
	editionID, _, _ := k.GetStringValue("EditionID")
	productName, _, _ := k.GetStringValue("ProductName")
	home := IsHomeSKUStrings(editionID, productName)
	if home {
		logger.Info("sku_home detected edition=%s product=%s", editionID, productName)
	}
	return home
}
