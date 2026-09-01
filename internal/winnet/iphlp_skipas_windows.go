//go:build windows

package winnet

import (
	"fmt"
	"net"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
)

var (
	procGetUnicastIpAddressEntry = iphlpapi.NewProc("GetUnicastIpAddressEntry")
	procSetUnicastIpAddressEntry = iphlpapi.NewProc("SetUnicastIpAddressEntry")
)

// unicastSkipAsSourceState 读取 ifIndex 上指定 IPv4 的 SkipAsSource。
func unicastSkipAsSourceState(ifIndex int, ip net.IP) (skip bool, ok bool, err error) {
	ip4 := ip.To4()
	if ip4 == nil || ifIndex <= 0 {
		return false, false, fmt.Errorf("unicastSkipAsSourceState: bad args")
	}
	row, err := getUnicastRow(ifIndex, ip4)
	if err != nil {
		return false, false, err
	}
	return row.SkipAsSource != 0, true, nil
}

func getUnicastRow(ifIndex int, ip4 net.IP) (*windows.MibUnicastIpAddressRow, error) {
	var row windows.MibUnicastIpAddressRow
	procInitializeUnicastIpAddressEntry.Call(uintptr(unsafe.Pointer(&row)))
	row.InterfaceIndex = uint32(ifIndex)
	sa := (*windows.RawSockaddrInet4)(unsafe.Pointer(&row.Address))
	sa.Family = windows.AF_INET
	copy(sa.Addr[:], ip4)
	r1, _, e1 := procGetUnicastIpAddressEntry.Call(uintptr(unsafe.Pointer(&row)))
	if r1 != 0 {
		if errno, ok := e1.(windows.Errno); ok && (errno == errorNotFound || errno == windows.ERROR_NOT_FOUND) {
			return nil, fmt.Errorf("GetUnicastIpAddressEntry: not found ip=%s ifIndex=%d", ip4, ifIndex)
		}
		return nil, fmt.Errorf("GetUnicastIpAddressEntry: %v code=%d", e1, r1)
	}
	return &row, nil
}

func setUnicastSkipAsSource(ifIndex int, ip net.IP, skip bool) error {
	ip4 := ip.To4()
	if ip4 == nil || ifIndex <= 0 {
		return fmt.Errorf("setUnicastSkipAsSource: bad args")
	}
	row, err := getUnicastRow(ifIndex, ip4)
	if err != nil {
		return err
	}
	if skip {
		row.SkipAsSource = 1
	} else {
		row.SkipAsSource = 0
	}
	r1, _, e1 := procSetUnicastIpAddressEntry.Call(uintptr(unsafe.Pointer(row)))
	if r1 != 0 {
		return fmt.Errorf("SetUnicastIpAddressEntry ip=%s skip=%v: %v code=%d", ip4, skip, e1, r1)
	}
	return nil
}

// ApplyPreferVPNSkipAsSource 用 IP Helper 刷新 VPN/137 的 SkipAsSource（软换主路径，零 PS）。
//
// 返回 method：noop — 已正确；iphlp — 已改写；调用方 PS fallback 仅在 err!=nil 时。
func ApplyPreferVPNSkipAsSource(ifIndex int, vpnIP string) (method string, err error) {
	start := time.Now()
	vpnIP = strings.TrimSpace(vpnIP)
	vpn := net.ParseIP(vpnIP).To4()
	if vpn == nil || ifIndex <= 0 {
		return "", fmt.Errorf("ApplyPreferVPNSkipAsSource: 参数无效")
	}
	ents, err := ListUnicastIPv4OnIfIndex(ifIndex)
	if err != nil {
		return "", err
	}
	var vpnSkip bool
	var vpnFound bool
	var has137 bool
	var skip137 bool
	for _, e := range ents {
		if e.IP.Equal(vpn) {
			vpnFound = true
			vpnSkip = e.SkipAsSource
		}
		if netutil.IPv4IsICSPrivate(e.IP) {
			has137 = true
			skip137 = e.SkipAsSource
		}
	}
	if !vpnFound {
		return "", fmt.Errorf("ApplyPreferVPNSkipAsSource: vpn %s 不在 ifIndex=%d", vpnIP, ifIndex)
	}
	if !has137 {
		return "", fmt.Errorf("ApplyPreferVPNSkipAsSource: 无 137 on ifIndex=%d", ifIndex)
	}
	if !preferSkipAsSourceNeedsUpdate(vpnSkip, has137, skip137) {
		logger.Info("prefer_vpn_skipas ifIndex=%d method=noop vpn=%s elapsed=%s", ifIndex, vpnIP, time.Since(start))
		return "noop", nil
	}
	if e := setUnicastSkipAsSource(ifIndex, vpn, false); e != nil {
		return "", e
	}
	for _, e := range ents {
		if !netutil.IPv4IsICSPrivate(e.IP) {
			continue
		}
		if e.SkipAsSource {
			continue
		}
		if e2 := setUnicastSkipAsSource(ifIndex, e.IP, true); e2 != nil {
			return "", e2
		}
	}
	logger.Info("prefer_vpn_skipas ifIndex=%d method=iphlp vpn=%s elapsed=%s", ifIndex, vpnIP, time.Since(start))
	if after, e := ListUnicastIPv4OnIfIndex(ifIndex); e == nil {
		for _, e := range after {
			if e.IP.Equal(vpn) || netutil.IPv4IsICSPrivate(e.IP) {
				logger.Info("ics_src_diag ip=%s prefix=%d skip=%v", e.IP, e.PrefixLen, e.SkipAsSource)
			}
		}
	}
	return "iphlp", nil
}
