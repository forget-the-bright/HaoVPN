//go:build windows

package winnet

import (
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

// unicastIPv4OnIfIndex 用 GetUnicastIpAddressTable(AF_INET) 列出指定 ifIndex 上的 IPv4。
//
// 比 net.InterfaceByIndex+Addrs 快：后者每次全表 GetAdaptersAddresses（公司机可数秒）。
// 调用方须 FreeMibTable；本函数已释放。
func unicastIPv4OnIfIndex(ifIndex int) ([]net.IP, error) {
	if ifIndex <= 0 {
		return nil, fmt.Errorf("unicastIPv4OnIfIndex: invalid ifIndex=%d", ifIndex)
	}
	var table *windows.MibUnicastIpAddressTable
	if err := windows.GetUnicastIpAddressTable(windows.AF_INET, &table); err != nil {
		return nil, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))

	out := make([]net.IP, 0, 4)
	n := int(table.NumEntries)
	// Table 为变长数组首元素；按 NumEntries 用 unsafe 切片
	rows := unsafe.Slice(&table.Table[0], n)
	for i := 0; i < n; i++ {
		row := &rows[i]
		if int(row.InterfaceIndex) != ifIndex {
			continue
		}
		ip := ipv4FromSockaddrInet(&row.Address)
		if ip != nil {
			out = append(out, ip)
		}
	}
	return out, nil
}

// ipv4FromSockaddrInet 从 SOCKADDR_INET 联合体取出 IPv4（Family=AF_INET）。
func ipv4FromSockaddrInet(sa *windows.RawSockaddrInet6) net.IP {
	if sa == nil {
		return nil
	}
	if sa.Family != windows.AF_INET {
		return nil
	}
	// sockaddr_in：Family(2)+Port(2)+Addr(4)；与 Inet6 联合体前 8 字节重叠
	type sockaddrIn struct {
		Family uint16
		Port   uint16
		Addr   [4]byte
	}
	in := (*sockaddrIn)(unsafe.Pointer(sa))
	return net.IPv4(in.Addr[0], in.Addr[1], in.Addr[2], in.Addr[3]).To4()
}

// interfaceHasIPv4ByIPHelper 用 MIB 表判断 ifIndex 是否已有目标 IPv4。
func interfaceHasIPv4ByIPHelper(ifIndex int, want net.IP) (bool, error) {
	ips, err := unicastIPv4OnIfIndex(ifIndex)
	if err != nil {
		return false, err
	}
	want4 := want.To4()
	if want4 == nil {
		return false, nil
	}
	for _, ip := range ips {
		if ip.Equal(want4) {
			return true, nil
		}
	}
	return false, nil
}

// interfaceHasICSPrivateByIPHelper 用 MIB 表判断 ifIndex 是否含 192.168.137.*。
func interfaceHasICSPrivateByIPHelper(ifIndex int) (bool, error) {
	ips, err := unicastIPv4OnIfIndex(ifIndex)
	if err != nil {
		return false, err
	}
	for _, ip := range ips {
		if IPv4IsICSPrivate(ip) {
			return true, nil
		}
	}
	return false, nil
}
