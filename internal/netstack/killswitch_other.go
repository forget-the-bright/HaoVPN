//go:build !windows

package netstack

import "fmt"

// KillSwitchSupported 非 Windows 不支持。
func KillSwitchSupported() error {
	return fmt.Errorf("杀开关仅支持 Windows（WFP）")
}

// EnableKillSwitch 非 Windows：硬失败。
func EnableKillSwitch(prefixes []string) error {
	return KillSwitchSupported()
}

// DisableKillSwitch 非 Windows 无操作。
func DisableKillSwitch() error {
	return nil
}

// RemoveKillSwitchRules 非 Windows 无操作。
func RemoveKillSwitchRules() error {
	return nil
}
