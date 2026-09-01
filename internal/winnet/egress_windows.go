//go:build windows

package winnet

import (
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"haovpn/internal/netutil"
	"haovpn/internal/logger"
)

// CollectEgressSnapshot 一次 GetAdaptersAddresses + GetIpForwardTable2；短缓存 30s。
//
// 日志：ics_egress snapshot elapsed= addrs= routes=（总采集耗时）。
func CollectEgressSnapshot() (*EgressSnapshot, error) {
	egressSnapMu.Lock()
	if egressSnapCache != nil && time.Since(egressSnapAt) < 30*time.Second {
		s := egressSnapCache
		egressSnapMu.Unlock()
		return s, nil
	}
	egressSnapMu.Unlock()

	start := time.Now()
	addrs, byIndex, err := listEgressAddrs()
	if err != nil {
		return nil, err
	}
	routes, err := listEgressRoutes(byIndex)
	if err != nil {
		return nil, err
	}
	snap := &EgressSnapshot{Addrs: addrs, Routes: routes, ByIndex: byIndex}
	logger.Info("ics_egress snapshot elapsed=%s addrs=%d routes=%d", time.Since(start), len(addrs), len(routes))

	egressSnapMu.Lock()
	egressSnapCache = snap
	egressSnapAt = time.Now()
	egressSnapMu.Unlock()
	return snap, nil
}

func listEgressAddrs() ([]EgressAddr, map[int]string, error) {
	var buf []byte
	size := uint32(15000)
	var err error
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		err = windows.GetAdaptersAddresses(windows.AF_INET,
			windows.GAA_FLAG_INCLUDE_PREFIX,
			0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		return nil, nil, err
	}
	if err != nil {
		return nil, nil, err
	}
	byIndex := make(map[int]string)
	var out []EgressAddr
	aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
	for ; aa != nil; aa = aa.Next {
		name := windows.UTF16PtrToString(aa.FriendlyName)
		if name == "" || netutil.IsVirtualInterfaceName(name) {
			continue
		}
		if aa.OperStatus != windows.IfOperStatusUp {
			continue
		}
		idx := int(aa.IfIndex)
		byIndex[idx] = name
		for u := aa.FirstUnicastAddress; u != nil; u = u.Next {
			ip := u.Address.IP()
			ip4 := ip.To4()
			if ip4 == nil || ip4.IsLoopback() || (ip4[0] == 169 && ip4[1] == 254) {
				continue
			}
			ipCopy := append(net.IP(nil), ip4...)
			out = append(out, EgressAddr{
				IfIndex:   idx,
				Name:      name,
				IP:        ipCopy,
				PrefixLen: int(u.OnLinkPrefixLength),
			})
		}
	}
	return out, byIndex, nil
}

func listEgressRoutes(byIndex map[int]string) ([]EgressRoute, error) {
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return nil, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	rows := table.Rows()
	out := make([]EgressRoute, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		idx := int(r.InterfaceIndex)
		if _, ok := byIndex[idx]; !ok {
			continue
		}
		dest := ipv4FromRawSockaddrInet(&r.DestinationPrefix.Prefix)
		if dest == nil {
			continue
		}
		plen := int(r.DestinationPrefix.PrefixLength)
		isDef := plen == 0 && dest.Equal(net.IPv4zero)
		out = append(out, EgressRoute{
			IfIndex:   idx,
			Dest:      dest,
			PrefixLen: plen,
			Metric:    r.Metric,
			IsDefault: isDef,
		})
	}
	return out, nil
}

func ipv4FromRawSockaddrInet(sa *windows.RawSockaddrInet) net.IP {
	if sa == nil {
		return nil
	}
	in := (*windows.RawSockaddrInet4)(unsafe.Pointer(sa))
	if in.Family != windows.AF_INET {
		return nil
	}
	return net.IPv4(in.Addr[0], in.Addr[1], in.Addr[2], in.Addr[3]).To4()
}
