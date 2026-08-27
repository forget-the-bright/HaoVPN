//go:build !windows

package winnet

// ResolveInterfaceAlias 非 Windows 无 netsh 别名差异，直接返回配置名。
func ResolveInterfaceAlias(configName string) string {
	return configName
}

// InterfaceIndex 非 Windows 不提供 Wintun ifIndex 解析。
func InterfaceIndex(name string) (int, error) {
	_ = name
	return 0, ErrNotSupported
}

// RegisterFromLUID 非 Windows 无 LUID 概念，无操作。
func RegisterFromLUID(configName string, luid uint64) {}

// InterfaceHasIPv4 非 Windows 桩实现，恒 false。
func InterfaceHasIPv4(configName string, ifIndex int, ip string) bool {
	_, _, _ = configName, ifIndex, ip
	return false
}

// ErrNotSupported 表示当前平台不支持 winnet 的 Windows 专有操作。
var ErrNotSupported = errorString("winnet: 仅 Windows 支持")

type errorString string

func (e errorString) Error() string { return string(e) }
