//go:build !windows

package netstack

import "fmt"

// KillSwitchSupported 非 Windows 平台不支持 WFP 杀开关。
//
// 返回：恒为 error，说明仅 Windows（WFP）实现。
func KillSwitchSupported() error {
	return fmt.Errorf("杀开关仅支持 Windows（WFP）")
}

// EnableKillSwitch 非 Windows 平台硬失败，避免误以为已阻断 AllowedIPs。
func EnableKillSwitch(prefixes []string) error {
	return KillSwitchSupported()
}

// DisableKillSwitch 非 Windows 无 WFP 规则，恒为无操作。
func DisableKillSwitch() error {
	return nil
}

// RemoveKillSwitchRules 非 Windows 无 WFP 引擎，恒为无操作。
func RemoveKillSwitchRules() error {
	return nil
}
