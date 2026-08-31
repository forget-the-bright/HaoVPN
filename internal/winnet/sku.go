package winnet

import (
	"strings"

	"haovpn/internal/netutil"
)

// IsHomeSKUStrings 根据 EditionID / ProductName 判断是否为家庭版/Core（无 Hyper-V/WinNAT）。
//
// 纯字符串逻辑，便于表驱动单测；Windows 运行时由 IsWindowsHomeSKU 读注册表后调用。
func IsHomeSKUStrings(editionID, productName string) bool {
	e := netutil.TrimLower(editionID)
	p := netutil.TrimLower(productName)
	if e != "" {
		// EditionID：Core / CoreN / CoreSingleLanguage / CoreCountrySpecific 等
		if strings.Contains(e, "core") || strings.Contains(e, "home") {
			return true
		}
	}
	if p != "" && strings.Contains(p, "home") {
		return true
	}
	return false
}
