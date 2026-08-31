//go:build !windows

package tun

// warmupPlatform 非 Windows 无 Wintun 冷创建问题，预热为空操作。
func warmupPlatform(name string) error {
	_ = name
	return nil
}
