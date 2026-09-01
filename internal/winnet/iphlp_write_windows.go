//go:build windows

package winnet

import (
	"fmt"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
)

var (
	procInitializeUnicastIpAddressEntry = iphlpapi.NewProc("InitializeUnicastIpAddressEntry")
	procCreateUnicastIpAddressEntry     = iphlpapi.NewProc("CreateUnicastIpAddressEntry")
	procDeleteUnicastIpAddressEntry     = iphlpapi.NewProc("DeleteUnicastIpAddressEntry")
	procCreateIpForwardEntry2           = iphlpapi.NewProc("CreateIpForwardEntry2")
	procDeleteIpForwardEntry2           = iphlpapi.NewProc("DeleteIpForwardEntry2")
	procInitializeIpForwardEntry        = iphlpapi.NewProc("InitializeIpForwardEntry")
	procConvertInterfaceIndexToLuid     = iphlpapi.NewProc("ConvertInterfaceIndexToLuid")
)

// errorObjectAlreadyExists Windows ERROR_OBJECT_ALREADY_EXISTS (5010)。
const errorObjectAlreadyExists windows.Errno = 5010

// errorNotFound Windows ERROR_NOT_FOUND (1168) — 删除路由时目标不存在视为成功。
const errorNotFound windows.Errno = 1168

// SetInterfaceIPv4OnIndex 优先替换式配 IP（删旧加新）；失败回退 netsh set address。
//
// 为何替换：Wintun Close 保留适配器，仅 Create 会导致在线改 VPN IP 后双地址残留。
// 有活 137：KeepICS + 强制 VPN 主机前缀 /24（对齐 ics_prefix_keep；禁止 /32 打死 NAT）。
func SetInterfaceIPv4OnIndex(ifIndex int, ifName, ip string, prefixLen int) error {
	start := time.Now()
	if UseIPHelperEnabled() && ifIndex > 0 {
		keepICS := false
		if ents, err := ListUnicastIPv4OnIfIndex(ifIndex); err == nil {
			for _, e := range ents {
				if netutil.IPv4IsICSPrivate(e.IP) {
					keepICS = true
					break
				}
			}
		}
		usePrefix := prefixLen
		if keepICS {
			usePrefix = 24
		}
		var removed []string
		var kept string
		var err error
		if keepICS {
			removed, kept, err = ReplaceInterfaceIPv4KeepICS(ifIndex, ip, usePrefix)
		} else {
			removed, kept, err = ReplaceInterfaceIPv4(ifIndex, ip, usePrefix)
		}
		if err == nil {
			logger.Info("assign_ip method=iphlp replace removed=%v kept=%s/%d elapsed=%s ifIndex=%d keep_ics=%v",
				removed, kept, usePrefix, time.Since(start), ifIndex, keepICS)
			return nil
		}
		logger.Warn("assign_ip method=iphlp replace fail elapsed=%s keep_ics=%v: %v，回退 netsh", time.Since(start), keepICS, err)
	}
	mask := prefixLenToMask(prefixLen)
	err := SetInterfaceIPv4(ifName, ip, mask)
	logger.Info("assign_ip method=netsh elapsed=%s ifName=%s err=%v", time.Since(start), ifName, err)
	return err
}

// ReplaceInterfaceIPv4 将 ifIndex 上的 IPv4 收敛为唯一 wantIP/prefixLen。
//
// 步骤：列出单播 → 删除 ≠ wantIP（含 137）→ 若 want 缺失或前缀不对则删重建。
// 返回：removed 被删地址列表；kept 最终保留的 wantIP；同 IP 且无多余时 removed 为空（快路径）。
// 软换 IP 且 ICS 仍在时用 ReplaceInterfaceIPv4KeepICS。
func ReplaceInterfaceIPv4(ifIndex int, wantIP string, prefixLen int) (removed []string, kept string, err error) {
	return replaceInterfaceIPv4(ifIndex, wantIP, prefixLen, false)
}

// ReplaceInterfaceIPv4KeepICS 同 ReplaceInterfaceIPv4，但保留 192.168.137.*（在线软换 VPN IP）。
func ReplaceInterfaceIPv4KeepICS(ifIndex int, wantIP string, prefixLen int) (removed []string, kept string, err error) {
	return replaceInterfaceIPv4(ifIndex, wantIP, prefixLen, true)
}

func replaceInterfaceIPv4(ifIndex int, wantIP string, prefixLen int, keepICS bool) (removed []string, kept string, err error) {
	want := net.ParseIP(wantIP).To4()
	if want == nil {
		return nil, "", fmt.Errorf("ReplaceInterfaceIPv4: invalid ipv4 %q", wantIP)
	}
	if ifIndex <= 0 {
		return nil, "", fmt.Errorf("ReplaceInterfaceIPv4: invalid ifIndex=%d", ifIndex)
	}
	if prefixLen < 0 || prefixLen > 32 {
		return nil, "", fmt.Errorf("ReplaceInterfaceIPv4: bad prefixLen=%d", prefixLen)
	}

	ents, err := unicastIPv4EntriesOnIfIndex(ifIndex)
	if err != nil {
		return nil, "", err
	}
	haveStr := make([]string, 0, len(ents))
	for _, e := range ents {
		haveStr = append(haveStr, e.IP.String())
	}
	var toRemove []string
	if keepICS {
		toRemove = netutil.IPv4AddrsToRemoveKeepICS(haveStr, want.String())
	} else {
		toRemove = netutil.IPv4AddrsToRemove(haveStr, want.String())
	}
	removed = append([]string{}, toRemove...)
	removeSet := make(map[string]struct{}, len(toRemove))
	for _, s := range toRemove {
		removeSet[s] = struct{}{}
	}

	for _, e := range ents {
		ipStr := e.IP.String()
		if ipStr == want.String() {
			continue
		}
		if _, ok := removeSet[ipStr]; !ok {
			continue // KeepICS：跳过 137
		}
		if err := deleteUnicastIPv4(ifIndex, e.IP); err != nil {
			return removed, "", fmt.Errorf("delete %s: %w", ipStr, err)
		}
	}

	// 再查表：want 缺失或前缀不对则删重建（VPN 通常 /32；ICS 曾把旧 VPN 改成 /24）
	ents2, err2 := unicastIPv4EntriesOnIfIndex(ifIndex)
	if err2 != nil {
		return removed, "", err2
	}
	wantPresent := false
	wantPrefixOK := false
	for _, e := range ents2 {
		if !e.IP.Equal(want) {
			continue
		}
		wantPresent = true
		if e.PrefixLen == prefixLen {
			wantPrefixOK = true
		} else {
			if err := deleteUnicastIPv4(ifIndex, e.IP); err != nil {
				return removed, "", fmt.Errorf("delete wrong-prefix %s/%d: %w", want, e.PrefixLen, err)
			}
			removed = append(removed, fmt.Sprintf("%s/%d", want.String(), e.PrefixLen))
			wantPresent = false
			wantPrefixOK = false
		}
	}
	if !wantPresent || !wantPrefixOK {
		if err := createUnicastIPv4(ifIndex, want.String(), prefixLen); err != nil {
			return removed, "", fmt.Errorf("create %s/%d: %w", want, prefixLen, err)
		}
	}
	return removed, want.String(), nil
}

func prefixLenToMask(ones int) string {
	if ones < 0 {
		ones = 0
	}
	if ones > 32 {
		ones = 32
	}
	var mask uint32
	if ones > 0 {
		mask = ^uint32(0) << uint(32-ones)
	}
	return fmt.Sprintf("%d.%d.%d.%d", byte(mask>>24), byte(mask>>16), byte(mask>>8), byte(mask))
}

func createUnicastIPv4(ifIndex int, ipStr string, prefixLen int) error {
	ip := net.ParseIP(ipStr).To4()
	if ip == nil {
		return fmt.Errorf("invalid ipv4: %s", ipStr)
	}
	var row windows.MibUnicastIpAddressRow
	procInitializeUnicastIpAddressEntry.Call(uintptr(unsafe.Pointer(&row)))
	row.InterfaceIndex = uint32(ifIndex)
	row.OnLinkPrefixLength = uint8(prefixLen)
	row.DadState = 4 // IpDadStatePreferred

	sa := (*windows.RawSockaddrInet4)(unsafe.Pointer(&row.Address))
	sa.Family = windows.AF_INET
	copy(sa.Addr[:], ip)

	r1, _, e1 := procCreateUnicastIpAddressEntry.Call(uintptr(unsafe.Pointer(&row)))
	if r1 == 0 {
		return nil
	}
	if errno, ok := e1.(windows.Errno); ok && errno == errorObjectAlreadyExists {
		return nil
	}
	if r1 == uintptr(errorObjectAlreadyExists) {
		return nil
	}
	return fmt.Errorf("CreateUnicastIpAddressEntry: %v code=%d", e1, r1)
}

// deleteUnicastIPv4 删除 ifIndex 上指定 IPv4；不存在视为成功。
func deleteUnicastIPv4(ifIndex int, ip net.IP) error {
	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("deleteUnicastIPv4: not ipv4")
	}
	var row windows.MibUnicastIpAddressRow
	procInitializeUnicastIpAddressEntry.Call(uintptr(unsafe.Pointer(&row)))
	row.InterfaceIndex = uint32(ifIndex)
	sa := (*windows.RawSockaddrInet4)(unsafe.Pointer(&row.Address))
	sa.Family = windows.AF_INET
	copy(sa.Addr[:], ip4)

	r1, _, e1 := procDeleteUnicastIpAddressEntry.Call(uintptr(unsafe.Pointer(&row)))
	if r1 == 0 {
		return nil
	}
	if errno, ok := e1.(windows.Errno); ok && (errno == errorNotFound || errno == windows.ERROR_NOT_FOUND) {
		return nil
	}
	if r1 == uintptr(errorNotFound) || r1 == uintptr(windows.ERROR_NOT_FOUND) {
		return nil
	}
	return fmt.Errorf("DeleteUnicastIpAddressEntry: %v code=%d", e1, r1)
}

// ifIndexToLuid 将 ifIndex 转为接口 LUID（CreateIpForwardEntry2 / DNS GUID 需要）。
func ifIndexToLuid(ifIndex int) (uint64, error) {
	if ifIndex <= 0 {
		return 0, fmt.Errorf("invalid ifIndex=%d", ifIndex)
	}
	var luid uint64
	r1, _, e1 := procConvertInterfaceIndexToLuid.Call(uintptr(uint32(ifIndex)), uintptr(unsafe.Pointer(&luid)))
	if r1 != 0 {
		if e1 != windows.ERROR_SUCCESS {
			return 0, fmt.Errorf("ConvertInterfaceIndexToLuid: %v code=%d", e1, r1)
		}
		return 0, fmt.Errorf("ConvertInterfaceIndexToLuid code=%d", r1)
	}
	return luid, nil
}

// setSockaddrInet4 将 IPv4 写入 SOCKADDR_INET 联合体。
func setSockaddrInet4(dst *windows.RawSockaddrInet, ip net.IP) {
	sa := (*windows.RawSockaddrInet4)(unsafe.Pointer(dst))
	*sa = windows.RawSockaddrInet4{}
	sa.Family = windows.AF_INET
	copy(sa.Addr[:], ip.To4())
}

// AddOnLinkRouteIPHelper 用官方 MIB_IPFORWARD_ROW2 + LUID 添加 on-link 路由。
// 失败返回 error 供调用方回退 route.exe。
func AddOnLinkRouteIPHelper(dest net.IP, prefixLen, ifIndex int) error {
	if !UseIPHelperEnabled() || ifIndex <= 0 {
		return fmt.Errorf("ip helper disabled")
	}
	dest4 := dest.To4()
	if dest4 == nil {
		return fmt.Errorf("dest not ipv4")
	}
	luid, err := ifIndexToLuid(ifIndex)
	if err != nil {
		return err
	}

	var row windows.MibIpForwardRow2
	procInitializeIpForwardEntry.Call(uintptr(unsafe.Pointer(&row)))
	row.InterfaceLuid = luid
	row.InterfaceIndex = uint32(ifIndex)
	setSockaddrInet4(&row.DestinationPrefix.Prefix, dest4)
	row.DestinationPrefix.PrefixLength = uint8(prefixLen)
	// NextHop 保持 AF_UNSPEC（Initialize 已清零）→ on-link
	row.Metric = 256
	row.Protocol = windows.MIB_IPPROTO_NETMGMT

	r1, _, e1 := procCreateIpForwardEntry2.Call(uintptr(unsafe.Pointer(&row)))
	if r1 == 0 {
		return nil
	}
	if errno, ok := e1.(windows.Errno); ok && errno == errorObjectAlreadyExists {
		return nil
	}
	if r1 == uintptr(errorObjectAlreadyExists) {
		return nil
	}
	return fmt.Errorf("CreateIpForwardEntry2: %v code=%d", e1, r1)
}

// DeleteOnLinkRouteIPHelper 用 DeleteIpForwardEntry2 删除 on-link 分流路由。
// 路由不存在（ERROR_NOT_FOUND）视为成功，供 Stop 路径幂等清理。
func DeleteOnLinkRouteIPHelper(dest net.IP, prefixLen, ifIndex int) error {
	if !UseIPHelperEnabled() || ifIndex <= 0 {
		return fmt.Errorf("ip helper disabled")
	}
	dest4 := dest.To4()
	if dest4 == nil {
		return fmt.Errorf("dest not ipv4")
	}
	luid, err := ifIndexToLuid(ifIndex)
	if err != nil {
		return err
	}

	var row windows.MibIpForwardRow2
	procInitializeIpForwardEntry.Call(uintptr(unsafe.Pointer(&row)))
	row.InterfaceLuid = luid
	row.InterfaceIndex = uint32(ifIndex)
	setSockaddrInet4(&row.DestinationPrefix.Prefix, dest4)
	row.DestinationPrefix.PrefixLength = uint8(prefixLen)
	row.Protocol = windows.MIB_IPPROTO_NETMGMT

	r1, _, e1 := procDeleteIpForwardEntry2.Call(uintptr(unsafe.Pointer(&row)))
	if r1 == 0 {
		return nil
	}
	if errno, ok := e1.(windows.Errno); ok && (errno == errorNotFound || errno == windows.ERROR_NOT_FOUND) {
		return nil
	}
	if r1 == uintptr(errorNotFound) || r1 == uintptr(windows.ERROR_NOT_FOUND) {
		return nil
	}
	return fmt.Errorf("DeleteIpForwardEntry2: %v code=%d", e1, r1)
}
