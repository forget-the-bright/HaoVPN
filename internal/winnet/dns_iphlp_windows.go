//go:build windows

package winnet

import (
	"fmt"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"haovpn/internal/logger"
)

var (
	procConvertInterfaceLuidToGuid = iphlpapi.NewProc("ConvertInterfaceLuidToGuid")
	procSetInterfaceDnsSettings    = iphlpapi.NewProc("SetInterfaceDnsSettings")
)

// DNS_INTERFACE_SETTINGS Version1（与 WireGuard winipcfg 对齐）。
const (
	dnsInterfaceSettingsVersion1     = 1
	dnsInterfaceSettingsFlagNameserver = 0x0002
)

// dnsInterfaceSettings 对应 netioapi DNS_INTERFACE_SETTINGS。
type dnsInterfaceSettings struct {
	Version              uint32
	_                    [4]byte
	Flags                uint64
	Domain               *uint16
	NameServer           *uint16
	SearchList           *uint16
	RegistrationEnabled  uint32
	RegisterAdapterName  uint32
	EnableLLMNR          uint32
	QueryAdapterName     uint32
	ProfileNameServer    *uint16
}

// SetInterfaceDNSServers 写入静态 DNS 列表（逗号分隔 NameServer）。
// ifIndex>0 时走 Index→LUID→GUID；否则尝试按 ifName 解析 ifIndex。
func SetInterfaceDNSServers(ifName string, ifIndex int, servers []string) error {
	start := time.Now()
	if len(servers) == 0 {
		return fmt.Errorf("dns servers empty")
	}
	if UseIPHelperEnabled() {
		idx := ifIndex
		if idx <= 0 && ifName != "" {
			if i, err := InterfaceIndex(ifName); err == nil {
				idx = i
			}
		}
		if idx > 0 {
			if err := setDNSViaIPHelper(idx, servers); err == nil {
				logger.Info("dns_set method=iphlp elapsed=%s ifIndex=%d servers=%v", time.Since(start), idx, servers)
				return nil
			} else {
				logger.Warn("dns_set method=iphlp fail elapsed=%s: %v，回退 netsh", time.Since(start), err)
			}
		}
	}
	// netsh：首条 set，其余 add
	if err := RunNetsh("interface", "ipv4", "set", "dnsservers", ifName,
		"source=static", "address="+servers[0], "register=none", "validate=no"); err != nil {
		logger.Info("dns_set method=netsh elapsed=%s ifName=%s err=%v", time.Since(start), ifName, err)
		return err
	}
	for i := 1; i < len(servers); i++ {
		if err := AddInterfaceDNS(ifName, servers[i], i+1); err != nil {
			logger.Info("dns_set method=netsh elapsed=%s ifName=%s err=%v", time.Since(start), ifName, err)
			return err
		}
	}
	logger.Info("dns_set method=netsh elapsed=%s ifName=%s servers=%v", time.Since(start), ifName, servers)
	return nil
}

func setDNSViaIPHelper(ifIndex int, servers []string) error {
	luid, err := ifIndexToLuid(ifIndex)
	if err != nil {
		return err
	}
	guid, err := luidToGUID(luid)
	if err != nil {
		return err
	}
	joined := strings.Join(servers, ",")
	ns, err := windows.UTF16PtrFromString(joined)
	if err != nil {
		return err
	}
	settings := dnsInterfaceSettings{
		Version:    dnsInterfaceSettingsVersion1,
		Flags:      dnsInterfaceSettingsFlagNameserver,
		NameServer: ns,
	}
	return callSetInterfaceDnsSettings(guid, &settings)
}

func luidToGUID(luid uint64) (windows.GUID, error) {
	var g windows.GUID
	r1, _, e1 := procConvertInterfaceLuidToGuid.Call(uintptr(unsafe.Pointer(&luid)), uintptr(unsafe.Pointer(&g)))
	if r1 != 0 {
		if e1 != windows.ERROR_SUCCESS {
			return g, fmt.Errorf("ConvertInterfaceLuidToGuid: %v code=%d", e1, r1)
		}
		return g, fmt.Errorf("ConvertInterfaceLuidToGuid code=%d", r1)
	}
	return g, nil
}

// callSetInterfaceDnsSettings GUID 按值传递；amd64 实际传指针，arm64/386 拆字。
// 参考 WireGuard winipcfg。
func callSetInterfaceDnsSettings(guid windows.GUID, settings *dnsInterfaceSettings) error {
	words := (*[4]uintptr)(unsafe.Pointer(&guid))
	var r1 uintptr
	var e1 error
	switch runtime.GOARCH {
	case "amd64":
		r1, _, e1 = procSetInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&guid)), uintptr(unsafe.Pointer(settings)))
	case "arm64":
		r1, _, e1 = procSetInterfaceDnsSettings.Call(words[0], words[1], uintptr(unsafe.Pointer(settings)))
	case "386", "arm":
		r1, _, e1 = procSetInterfaceDnsSettings.Call(words[0], words[1], words[2], words[3], uintptr(unsafe.Pointer(settings)))
	default:
		return fmt.Errorf("SetInterfaceDnsSettings: unsupported GOARCH=%s", runtime.GOARCH)
	}
	if r1 == 0 {
		return nil
	}
	if e1 != windows.ERROR_SUCCESS {
		return fmt.Errorf("SetInterfaceDnsSettings: %v code=%d", e1, r1)
	}
	return fmt.Errorf("SetInterfaceDnsSettings code=%d", r1)
}

// AddInterfaceDNS 向接口追加次级 DNS（netsh；IP Helper 一次写全列表时不经此路径）。
func AddInterfaceDNS(ifName, server string, index int) error {
	start := time.Now()
	err := RunNetsh("interface", "ipv4", "add", "dnsservers", ifName, server,
		"index="+fmt.Sprintf("%d", index), "validate=no")
	logger.Info("dns_add method=netsh elapsed=%s ifName=%s err=%v", time.Since(start), ifName, err)
	return err
}
