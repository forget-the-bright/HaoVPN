//go:build windows

package tun

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"

	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/winnet"
)

// haovpnWintunGUID 固定 GUID，使同 WintunPool 适配器在 Windows 上身份稳定，减少重命名孤儿网卡。
var haovpnWintunGUID = windows.GUID{
	Data1: 0x8A4F2C1E,
	Data2: 0x5B3D,
	Data3: 0x4E9A,
	Data4: [8]byte{0x9F, 0x2A, 0x7C, 0x10, 0x48, 0x56, 0x50, 0x4E},
}

// prepareWintunAdapter 启动前清理 Windows 上因重名产生的 Wintun 孤儿网卡（如 haovpn0 1）。
//
// 参数：configName — yaml tun.name，须与 Wintun OpenAdapter 名一致。
// 返回：PowerShell 执行失败时 error；无孤儿网卡时 nil。
// 副作用：可能 Remove-NetAdapter 删除后缀名网卡；写 Info/Debug 日志。
func prepareWintunAdapter(configName string) error {
	if configName == "" {
		return nil
	}
	ps := buildPrepareWintunPSScript(configName)
	out, err := winnet.RunPS(ps)
	trimmed := strings.TrimSpace(string(out))
	if trimmed != "" {
		logger.Info("Wintun 启动前清理: %s", trimmed)
	}
	if err != nil {
		return fmt.Errorf("清理 Wintun 孤儿网卡: %w", err)
	}
	return nil
}

// buildPrepareWintunPSScript 生成清理孤儿 Wintun 网卡的 PowerShell 脚本（供单测校验）。
func buildPrepareWintunPSScript(configName string) string {
	return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$want = '%s'
$removed = @()
Get-NetAdapter -ErrorAction SilentlyContinue | Where-Object {
  $_.InterfaceDescription -match 'Wintun|%s' -and
  $_.Name -ne $want -and
  ($_.Name -like ($want + '*'))
} | ForEach-Object {
  Remove-NetAdapter -Name $_.Name -Confirm:$false -ErrorAction Stop
  $removed += $_.Name
}
if ($removed.Count -gt 0) {
  Write-Output ('已移除孤儿网卡: ' + ($removed -join ', '))
}
`, winnet.EscapeSingleQuoted(configName), brand.WintunPool)
}

// openWintunAdapter 优先 Open 已有适配器；失败则清理孤儿后 Create（保留供下次复用）。
func openWintunAdapter(name string) (*wintun.Adapter, bool, error) {
	installWintunLogger()

	adapter, err := wintun.OpenAdapter(name)
	if err == nil {
		logger.Debug("Wintun OpenAdapter 成功: %s", name)
		return adapter, true, nil
	}
	logger.Debug("Wintun OpenAdapter 未命中 %s: %v，尝试清理后 Create", name, err)

	if err := prepareWintunAdapter(name); err != nil {
		logger.Warn("Wintun 孤儿网卡清理失败（继续 Create）: %v", err)
	}

	adapter, err = wintun.OpenAdapter(name)
	if err == nil {
		logger.Info("Wintun 清理后复用适配器: %s", name)
		return adapter, true, nil
	}

	adapter, err = wintun.CreateAdapter(name, brand.WintunPool, &haovpnWintunGUID)
	if err != nil {
		return nil, false, fmt.Errorf("wintun open/create: %w", err)
	}
	logger.Debug("Wintun CreateAdapter 新建: %s", name)
	return adapter, false, nil
}
