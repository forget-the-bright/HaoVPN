//go:build windows

package netstack

import (
	"net"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/platform"
	"haovpn/internal/winnet"
)

// ifIndex 解析已统一到 internal/winnet（避免 netstack→tun 反向依赖）。

// addClientRoutePlatform 添加分流路由：优先 IP Helper on-link，失败回退 route.exe。
func addClientRoutePlatform(cidr, tunName, gateway string) error {
	_ = gateway
	start := time.Now()
	dest, mask, err := netutil.SplitCIDR(cidr)
	if err != nil {
		return err
	}
	ifIndex, err := winnet.InterfaceIndex(tunName)
	if err != nil {
		return err
	}
	ones := 32
	if _, ipNet, perr := net.ParseCIDR(cidr); perr == nil {
		ones, _ = ipNet.Mask.Size()
	}
	ip := net.ParseIP(dest)
	if winnet.UseIPHelperEnabled() && ip != nil {
		if err := winnet.AddOnLinkRouteIPHelper(ip, ones, ifIndex); err == nil {
			logger.Info("route_add cidr=%s elapsed=%s method=iphlp", cidr, time.Since(start))
			return nil
		} else {
			logger.Warn("route_add method=iphlp fail cidr=%s elapsed=%s: %v，回退 route.exe", cidr, time.Since(start), err)
		}
	}
	args := WindowsOnLinkRouteArgs(dest, mask, ifIndex)
	cmd := platform.Command("route", args...)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "对象已存在") || strings.Contains(strings.ToLower(msg), "exists") {
			logger.Info("route_add cidr=%s elapsed=%s method=route_exe result=exists", cidr, elapsed)
			return nil
		}
		logger.Warn("route_add cidr=%s elapsed=%s method=route_exe err=%v", cidr, elapsed, err)
		return platform.CommandOutputError("route "+strings.Join(args, " "), out, err)
	}
	logger.Info("route_add cidr=%s elapsed=%s method=route_exe", cidr, elapsed)
	return nil
}

// delClientRoutePlatform 删除分流路由：优先 IP Helper，失败回退 route.exe；不存在不阻断。
func delClientRoutePlatform(cidr, tunName, gateway string) error {
	start := time.Now()
	dest, mask, err := netutil.SplitCIDR(cidr)
	if err != nil {
		return err
	}
	_ = gateway
	ones := 32
	if _, ipNet, perr := net.ParseCIDR(cidr); perr == nil {
		ones, _ = ipNet.Mask.Size()
	}
	ip := net.ParseIP(dest)
	ifIndex, idxErr := winnet.InterfaceIndex(tunName)
	if winnet.UseIPHelperEnabled() && ip != nil && idxErr == nil && ifIndex > 0 {
		if err := winnet.DeleteOnLinkRouteIPHelper(ip, ones, ifIndex); err == nil {
			logger.Info("route_del cidr=%s elapsed=%s method=iphlp", cidr, time.Since(start))
			return nil
		} else {
			logger.Warn("route_del method=iphlp fail cidr=%s elapsed=%s: %v，回退 route.exe", cidr, time.Since(start), err)
		}
	}
	cmd := platform.Command("route", "DELETE", dest, "MASK", mask)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		logger.Warn("route_del cidr=%s elapsed=%s method=route_exe err=%v out=%s", cidr, elapsed, err, strings.TrimSpace(string(out)))
	} else {
		logger.Info("route_del cidr=%s elapsed=%s method=route_exe", cidr, elapsed)
	}
	return nil
}
