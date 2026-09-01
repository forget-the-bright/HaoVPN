//go:build !windows

package winnet

// HasWintunOrphanAdapters 非 Windows 恒 false。
func HasWintunOrphanAdapters(configName string) bool {
	_ = configName
	return false
}
