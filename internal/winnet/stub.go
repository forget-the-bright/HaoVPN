//go:build !windows

package winnet

import (
	"context"
)

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

// ListUnicastIPv4OnIfIndex 非 Windows 返回空。
func ListUnicastIPv4OnIfIndex(ifIndex int) ([]UnicastIPv4Entry, error) {
	_ = ifIndex
	return nil, nil
}

// ReplaceInterfaceIPv4 非 Windows 不支持。
func ReplaceInterfaceIPv4(ifIndex int, wantIP string, prefixLen int) (removed []string, kept string, err error) {
	_, _, _ = ifIndex, wantIP, prefixLen
	return nil, "", ErrNotSupported
}

// ReplaceInterfaceIPv4KeepICS 非 Windows 不支持。
func ReplaceInterfaceIPv4KeepICS(ifIndex int, wantIP string, prefixLen int) (removed []string, kept string, err error) {
	_, _, _ = ifIndex, wantIP, prefixLen
	return nil, "", ErrNotSupported
}

// DeleteDefaultRouteOnInterface 非 Windows 无操作。
func DeleteDefaultRouteOnInterface(ifIndex int, mode DefaultRouteScrubMode) (removed bool, err error) {
	_, _ = ifIndex, mode
	return false, nil
}

// PreferVPNAfterSoftIPReplace 非 Windows 无操作。
func PreferVPNAfterSoftIPReplace(ctx context.Context, configName string, ifIndex int, vpnIP string) error {
	_, _, _, _ = ctx, configName, ifIndex, vpnIP
	return nil
}

// ApplyPreferVPNSkipAsSource 非 Windows 无操作。
func ApplyPreferVPNSkipAsSource(ifIndex int, vpnIP string) (method string, err error) {
	_, _ = ifIndex, vpnIP
	return "noop", nil
}

// PreferVPNSourceWithICSContext 非 Windows 无操作。
func PreferVPNSourceWithICSContext(ctx context.Context, configName, vpnIP string) error {
	_, _, _ = ctx, configName, vpnIP
	return nil
}

// RemoveICSAddressesKeepVPN 非 Windows 无操作。
func RemoveICSAddressesKeepVPN(configName, vpnIP string) error {
	_, _ = configName, vpnIP
	return nil
}

// DisableAllICSContext 非 Windows 无操作。
func DisableAllICSContext(ctx context.Context) { _ = ctx }

// DisableICSPairContext 非 Windows 无操作。
func DisableICSPairContext(ctx context.Context, public, private string) {
	_, _, _ = ctx, public, private
}

// RunPSBestEffort 非 Windows 无操作（无 PowerShell）。
func RunPSBestEffort(script, opName string) {
	_, _ = script, opName
}

// RunPSBestEffortContext 非 Windows 无操作。
func RunPSBestEffortContext(ctx context.Context, script, opName string) {
	_, _, _ = ctx, script, opName
}

// RunPSOneShot 非 Windows 返回 ErrNotSupported。
func RunPSOneShot(script string) ([]byte, error) {
	_ = script
	return nil, ErrNotSupported
}

// RunPSOneShotContext 非 Windows 返回 ErrNotSupported。
func RunPSOneShotContext(ctx context.Context, script string) ([]byte, error) {
	_, _ = ctx, script
	return nil, ErrNotSupported
}

// HasICSResidue 非 Windows 恒 false（无 ICS）。
func HasICSResidue(configName string) bool {
	_ = configName
	return false
}

// CleanupICSResidue 非 Windows 无操作。
func CleanupICSResidue(configName, vpnIP string) error {
	_, _ = configName, vpnIP
	return nil
}

// CleanupICSResidueContext 非 Windows 无操作。
func CleanupICSResidueContext(ctx context.Context, configName, vpnIP string) error {
	_, _, _ = ctx, configName, vpnIP
	return nil
}

// DisableICSSessionContext 非 Windows 无操作。
func DisableICSSessionContext(ctx context.Context) { _ = ctx }

// RestoreInterfaceDNSDHCP 非 Windows 无操作。
func RestoreInterfaceDNSDHCP(ifName string) error {
	_ = ifName
	return nil
}

// ErrNotSupported 表示当前平台不支持 winnet 的 Windows 专有操作。
var ErrNotSupported = errorString("winnet: 仅 Windows 支持")

type errorString string

func (e errorString) Error() string { return string(e) }
