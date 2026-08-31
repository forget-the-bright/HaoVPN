//go:build windows

package netstack

import (
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/platform"
	"haovpn/internal/winnet"
)

// ifIndex 解析已统一到 internal/winnet（避免 netstack→tun 反向依赖）。

// addClientRoutePlatform 添加分流路由：经 Wintun 接口 on-link（忽略 gateway 作下一跳）。
func addClientRoutePlatform(cidr, tunName, gateway string) error {
	_ = gateway
	dest, mask, err := netutil.SplitCIDR(cidr)
	if err != nil {
		return err
	}
	ifIndex, err := winnet.InterfaceIndex(tunName)
	if err != nil {
		return err
	}
	args := WindowsOnLinkRouteArgs(dest, mask, ifIndex)
	cmd := platform.Command("route", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		// 已存在则视为成功（重连/重复应用策略）
		if strings.Contains(msg, "对象已存在") || strings.Contains(strings.ToLower(msg), "exists") {
			return nil
		}
		return platform.CommandOutputError("route "+strings.Join(args, " "), out, err)
	}
	return nil
}

// delClientRoutePlatform 删除分流路由；路由不存在时 Windows 常非零，记 Warn 不阻断。
func delClientRoutePlatform(cidr, tunName, gateway string) error {
	dest, mask, err := netutil.SplitCIDR(cidr)
	if err != nil {
		return err
	}
	_ = tunName
	_ = gateway
	cmd := platform.Command("route", "DELETE", dest, "MASK", mask)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Warn("route DELETE 失败 cidr=%s dest=%s: %v out=%s", cidr, dest, err, strings.TrimSpace(string(out)))
	}
	return nil
}
