//go:build !windows

package winnet

// IsWindowsHomeSKU 非 Windows 恒 false。
func IsWindowsHomeSKU() bool { return false }
