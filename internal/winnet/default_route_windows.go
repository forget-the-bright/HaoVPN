//go:build windows

package winnet

import (
	"fmt"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"haovpn/internal/logger"
)

// hasDefaultRouteOnInterface 查 IPv4 路由表，TUN ifIndex 上是否存在 0.0.0.0/0（包内）。
func hasDefaultRouteOnInterface(ifIndex int) (bool, error) {
	if ifIndex <= 0 {
		return false, fmt.Errorf("hasDefaultRouteOnInterface: bad ifIndex=%d", ifIndex)
	}
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return false, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	for i := range table.Rows() {
		r := &table.Rows()[i]
		dest := ipv4FromRawSockaddrInet(&r.DestinationPrefix.Prefix)
		if IsIPv4DefaultRouteOnIf(ifIndex, int(r.InterfaceIndex), dest, int(r.DestinationPrefix.PrefixLength)) {
			return true, nil
		}
	}
	return false, nil
}

// DeleteDefaultRouteOnInterface 删除指定网卡上的 IPv4 默认路由（0.0.0.0/0）。
//
// 仅查表 + IP Helper：无路由 skip；iphlp 删成功即返回。不起 PowerShell
//（ICS 后 Prefer 已紧跟 Enable，无需 Late PS 纵深；曾用的 ScrubDefaultRouteLate 已删除）。
func DeleteDefaultRouteOnInterface(ifIndex int) (removed bool, err error) {
	if ifIndex <= 0 {
		return false, fmt.Errorf("DeleteDefaultRouteOnInterface: bad ifIndex=%d", ifIndex)
	}
	start := time.Now()
	has, err := hasDefaultRouteOnInterface(ifIndex)
	if err != nil {
		logger.Debug("tun_default_route_scrub ifIndex=%d has_check err=%v", ifIndex, err)
	} else if !has {
		logger.Info("tun_default_route_scrub ifIndex=%d method=skip reason=none elapsed=%s", ifIndex, time.Since(start))
		return false, nil
	}

	if UseIPHelperEnabled() {
		if e := DeleteOnLinkRouteIPHelper(net.IPv4zero, 0, ifIndex); e != nil {
			logger.Debug("tun_default_route_scrub ifIndex=%d method=iphlp err=%v", ifIndex, e)
		} else {
			still, _ := hasDefaultRouteOnInterface(ifIndex)
			if !still {
				logger.Info("tun_default_route_scrub ifIndex=%d method=iphlp removed=true elapsed=%s", ifIndex, time.Since(start))
				return true, nil
			}
			logger.Debug("tun_default_route_scrub ifIndex=%d method=iphlp still_present", ifIndex)
		}
	}

	logger.Info("tun_default_route_scrub ifIndex=%d method=iphlp_only removed=false elapsed=%s", ifIndex, time.Since(start))
	return false, nil
}
