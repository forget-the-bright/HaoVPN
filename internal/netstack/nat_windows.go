//go:build windows

package netstack

import (
	"context"
	"net"
	"strings"
	"sync"

	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/platform"
	"haovpn/internal/safeutil"
	"haovpn/internal/winnet"
)

var (
	winnatMu                sync.Mutex
	winnatUnavailableCached bool // 本进程已确认无 WinNAT（家庭版等），跳过重复 PS
)

// setupNATForLANs 为 VPN 子网访问若干 LAN 配置 SNAT（Windows）。
//
// WinNAT：对 VPN 子网建一条 New-NetNat（与 LAN 条数无关）。
// ICS 回退：整表只 Enable 一次；与首条出站网卡相同的网段一并生效，异网卡跳过并 Warn（ics_multi_nic）。
func setupNATForLANs(ctx context.Context, vpnSubnet string, lanCIDRs []string, tunName string, tunIP net.IP, outboundIf string) (NATSetupOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := safeutil.Check(ctx); err != nil {
		return NATSetupOutcome{}, err
	}
	if len(lanCIDRs) == 0 {
		return NATSetupOutcome{}, nil
	}
	// 家庭版/Core：无 Hyper-V WinNAT，跳过 Get-NetNat/New-NetNat。
	if winnet.IsWindowsHomeSKU() {
		markWinNATUnavailable()
		logger.Info("WinNAT skip reason=sku_home，直接 ICS 回退")
		return setupICSForLANs(ctx, tunName, lanCIDRs, outboundIf, tunIP)
	}
	if isWinNATCachedUnavailable() {
		logger.Info("WinNAT 本进程已确认不可用，跳过 New-NetNat，直接 ICS 回退")
		return setupICSForLANs(ctx, tunName, lanCIDRs, outboundIf, tunIP)
	}
	winErr := setupWinNAT(vpnSubnet)
	if winErr == nil {
		logger.Info("windows: WinNAT 已覆盖 VPN 子网 %s（多 LAN 共用一条 NetNat） lans=%d", vpnSubnet, len(lanCIDRs))
		return NATSetupOutcome{}, nil
	}
	if isWinNATUnavailable(winErr) {
		markWinNATUnavailable()
		logger.Warn("WinNAT 不可用（Windows 家庭版或未启用 Hyper-V）: %v", winErr)
		logger.Info("尝试 ICS 回退（Internet 连接共享）…")
		return setupICSForLANs(ctx, tunName, lanCIDRs, outboundIf, tunIP)
	}
	return NATSetupOutcome{}, winErr
}

func isWinNATCachedUnavailable() bool {
	winnatMu.Lock()
	defer winnatMu.Unlock()
	return winnatUnavailableCached
}

func markWinNATUnavailable() {
	winnatMu.Lock()
	winnatUnavailableCached = true
	winnatMu.Unlock()
}

// resetWinNATCacheForTest 单测重置缓存。
func resetWinNATCacheForTest() {
	winnatMu.Lock()
	winnatUnavailableCached = false
	winnatMu.Unlock()
}

// setupWinNAT 使用 New-NetNat（依赖 Hyper-V/WinNAT 子系统）。
//
// 若已有同名同 prefix 规则则跳过；否则尽力 Remove 旧规则再 New。
// PowerShell 一律经 winnet.RunPSOneShot / RunPSBestEffort。
// 安全：name/vpnSubnet 必须 EscapeSingleQuoted 后嵌入 '…'，禁止裸 %s（握手下发子网若含 ' 可破坏脚本）。
func setupWinNAT(vpnSubnet string) error {
	name := brand.WinNATName
	if winNATMatches(name, vpnSubnet) {
		logger.Info("windows: NetNat %s 已存在 prefix=%s，跳过", name, vpnSubnet)
		return nil
	}
	winnet.RunPSBestEffort(winnet.PSSnippetRemoveNetNat(name), "Remove-NetNat-before-New")

	out, err := winnet.RunPSOneShot(winnet.PSSnippetNewNetNat(name, vpnSubnet))
	if err != nil {
		return platform.CommandOutputError("New-NetNat", out, err)
	}
	logger.Info("windows: New-NetNat %s prefix=%s", name, vpnSubnet)
	return nil
}

// isWinNATUnavailable 判断是否为 WinNAT 子系统缺失（Invalid class / 0x80041010）。
func isWinNATUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "0x80041010") ||
		strings.Contains(msg, "Invalid class") ||
		strings.Contains(msg, "无效") ||
		strings.Contains(msg, "Provider load failure") ||
		strings.Contains(msg, "0x80041013")
}

func teardownNATPlatform(vpnSubnet, lanCIDR, tunName string) error {
	_ = vpnSubnet
	_ = lanCIDR
	_ = tunName
	// 已确认无 WinNAT 时跳过无意义的 Remove-NetNat。
	if isWinNATCachedUnavailable() {
		return nil
	}
	winnet.RunPSBestEffort(winnet.PSSnippetRemoveNetNat(brand.WinNATName), "Remove-NetNat-teardown")
	return nil
}

// winNATMatches 检查是否已有相同 prefix 的 NetNat 规则（避免每次重启 Remove+New）。
// 一律 RunPSOneShot：Get-NetNat 等 CIM 脚本禁止走已删除的常驻主机。
func winNATMatches(name, prefix string) bool {
	if isWinNATCachedUnavailable() {
		return false
	}
	ps := winnet.PSSnippetGetNetNatMatch(name, prefix)
	out, err := winnet.RunPSOneShot(ps)
	if err != nil {
		return false
	}
	return parseWinNATMatchOutput(string(out))
}

// parseWinNATMatchOutput 解析 winNATMatches 脚本 stdout（表驱动单测）。
func parseWinNATMatchOutput(out string) bool {
	return strings.Contains(strings.ToUpper(strings.TrimSpace(out)), "MATCH")
}
