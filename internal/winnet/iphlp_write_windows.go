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

var (
	procInitializeUnicastIpAddressEntry = iphlpapi.NewProc("InitializeUnicastIpAddressEntry")
	procCreateUnicastIpAddressEntry     = iphlpapi.NewProc("CreateUnicastIpAddressEntry")
	procCreateIpForwardEntry2           = iphlpapi.NewProc("CreateIpForwardEntry2")
	procInitializeIpForwardEntry        = iphlpapi.NewProc("InitializeIpForwardEntry")
	procConvertInterfaceIndexToLuid     = iphlpapi.NewProc("ConvertInterfaceIndexToLuid")
)

// errorObjectAlreadyExists Windows ERROR_OBJECT_ALREADY_EXISTS (5010)。
const errorObjectAlreadyExists windows.Errno = 5010

// SetInterfaceIPv4OnIndex 优先 CreateUnicastIpAddressEntry；失败回退 netsh。
func SetInterfaceIPv4OnIndex(ifIndex int, ifName, ip string, prefixLen int) error {
	start := time.Now()
	if UseIPHelperEnabled() && ifIndex > 0 {
		if err := createUnicastIPv4(ifIndex, ip, prefixLen); err == nil {
			logger.Info("assign_ip method=iphlp elapsed=%s ifIndex=%d ip=%s/%d", time.Since(start), ifIndex, ip, prefixLen)
			return nil
		} else {
			logger.Warn("assign_ip method=iphlp fail elapsed=%s: %v，回退 netsh", time.Since(start), err)
		}
	}
	mask := prefixLenToMask(prefixLen)
	err := SetInterfaceIPv4(ifName, ip, mask)
	logger.Info("assign_ip method=netsh elapsed=%s ifName=%s err=%v", time.Since(start), ifName, err)
	return err
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
