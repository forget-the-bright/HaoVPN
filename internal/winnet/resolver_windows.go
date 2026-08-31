//go:build windows

package winnet

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"haovpn/internal/logger"
)

var (
	iphlpapi                        = windows.NewLazySystemDLL("iphlpapi.dll")
	procConvertInterfaceLuidToIndex = iphlpapi.NewProc("ConvertInterfaceLuidToIndex")
	procConvertInterfaceLuidToNameW = iphlpapi.NewProc("ConvertInterfaceLuidToNameW")
)

var (
	resolverMu sync.RWMutex
	byName     = map[string]entry{} // 配置名 → ifIndex + netsh 别名
)

type entry struct {
	ifIndex int
	alias   string
}

// RegisterFromLUID 在 TUN 设备打开后登记「配置名 → ifIndex / netsh 别名」映射。
//
// 参数：
//   configName — client.yaml tun.name 等逻辑名。
//   luid — Wintun 返回的接口 LUID；0 或空名时忽略。
// 副作用：写入包内 byName 缓存，供路由/DNS/netsh 复用。
// 上游：internal/tun 在 Open 成功后调用。
func RegisterFromLUID(configName string, luid uint64) {
	if configName == "" || luid == 0 {
		return
	}
	idx, err := luidToIfIndex(luid)
	if err != nil {
		return
	}
	alias, _ := luidToAlias(luid)
	if alias == "" {
		alias = configName
	}
	resolverMu.Lock()
	byName[configName] = entry{ifIndex: idx, alias: alias}
	resolverMu.Unlock()
}

// InterfaceIndex 按 TUN 配置名解析 Windows 接口索引（ifIndex）。
//
// 解析顺序：LUID 缓存 → net.InterfaceByName → PowerShell Get-NetAdapter 回退。
// 返回：正整数 ifIndex；找不到网卡时 error。
func InterfaceIndex(name string) (int, error) {
	if idx, ok := cachedIfIndex(name); ok {
		return idx, nil
	}
	if iface, err := net.InterfaceByName(name); err == nil && iface.Index > 0 {
		cacheEntry(name, iface.Index, name)
		return iface.Index, nil
	}
	idx, err := findAdapterIfIndexPS(name)
	if err != nil {
		return 0, err
	}
	cacheEntry(name, idx, ResolveInterfaceAlias(name))
	return idx, nil
}

// ResolveInterfaceAlias 返回 netsh 命令可用的接口别名（Friendly Name）。
//
// 参数：configName — 逻辑配置名；缓存未命中时原样返回 configName。
// 说明：Wintun 系统别名常与 yaml name 不同，须经 LUID 解析后 netsh 才生效。
func ResolveInterfaceAlias(configName string) string {
	resolverMu.RLock()
	e, ok := byName[configName]
	resolverMu.RUnlock()
	if ok && e.alias != "" {
		return e.alias
	}
	return configName
}

// InterfaceHasIPv4 检查指定网卡是否已绑定目标 IPv4 地址。
//
// 参数：
//   configName — 用于缓存回退与按名查网卡。
//   ifIndex — 已知索引时可 >0 加速；≤0 时从缓存或按名解析。
//   ipStr — 期望的 IPv4 字符串。
// 返回：已绑定且与 ipStr 相等时为 true。
//
// 性能：默认 IP Helper GetUnicastIpAddressTable（O(表)）；失败才 net.InterfaceByIndex。
// 有 ifIndex 时不用 ByName，避免二次慢路径。
func InterfaceHasIPv4(configName string, ifIndex int, ipStr string) bool {
	want := net.ParseIP(ipStr)
	if want == nil {
		return false
	}
	if ifIndex <= 0 {
		if idx, ok := cachedIfIndex(configName); ok {
			ifIndex = idx
		}
	}
	if ifIndex > 0 {
		if UseIPHelperEnabled() {
			start := time.Now()
			ok, err := interfaceHasIPv4ByIPHelper(ifIndex, want)
			if err == nil {
				logger.Debug("InterfaceHasIPv4 method=iphlp elapsed=%s ifIndex=%d hit=%v", time.Since(start), ifIndex, ok)
				return ok
			}
			logger.Debug("InterfaceHasIPv4 method=net_fallback elapsed=%s ifIndex=%d err=%v", time.Since(start), ifIndex, err)
		}
		return interfaceHasIPv4ByIndex(ifIndex, want)
	}
	return interfaceHasIPv4ByName(configName, want)
}

func cachedIfIndex(name string) (int, bool) {
	resolverMu.RLock()
	e, ok := byName[name]
	resolverMu.RUnlock()
	if ok && e.ifIndex > 0 {
		return e.ifIndex, true
	}
	return 0, false
}

func cacheEntry(name string, idx int, alias string) {
	if name == "" || idx <= 0 {
		return
	}
	resolverMu.Lock()
	byName[name] = entry{ifIndex: idx, alias: alias}
	resolverMu.Unlock()
}

func luidToIfIndex(luid uint64) (int, error) {
	var idx uint32
	r1, _, err := procConvertInterfaceLuidToIndex.Call(
		uintptr(unsafe.Pointer(&luid)),
		uintptr(unsafe.Pointer(&idx)),
	)
	if r1 != 0 {
		if err != syscall.Errno(0) {
			return 0, fmt.Errorf("ConvertInterfaceLuidToIndex: %w", err)
		}
		return 0, fmt.Errorf("ConvertInterfaceLuidToIndex: code=%d", r1)
	}
	if idx == 0 {
		return 0, fmt.Errorf("ConvertInterfaceLuidToIndex: 无效索引")
	}
	return int(idx), nil
}

func luidToAlias(luid uint64) (string, error) {
	buf := make([]uint16, 256)
	r1, _, err := procConvertInterfaceLuidToNameW.Call(
		uintptr(unsafe.Pointer(&luid)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if r1 != 0 {
		if err != syscall.Errno(0) {
			return "", fmt.Errorf("ConvertInterfaceLuidToNameW: %w", err)
		}
		return "", fmt.Errorf("ConvertInterfaceLuidToNameW: code=%d", r1)
	}
	alias := windows.UTF16ToString(buf)
	if alias == "" {
		return "", fmt.Errorf("ConvertInterfaceLuidToNameW: 空别名")
	}
	return alias, nil
}

// interfaceHasIPv4ByIndex 按 ifIndex 查地址；用 InterfaceByIndex，禁止 Interfaces() 全表。
func interfaceHasIPv4ByIndex(ifIndex int, want net.IP) bool {
	iface, err := net.InterfaceByIndex(ifIndex)
	if err != nil {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	return addrsContainIPv4(addrs, want)
}

func interfaceHasIPv4ByName(name string, want net.IP) bool {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	return addrsContainIPv4(addrs, want)
}

func addrsContainIPv4(addrs []net.Addr, want net.IP) bool {
	for _, a := range addrs {
		switch v := a.(type) {
		case *net.IPNet:
			if v.IP.To4() != nil && v.IP.Equal(want) {
				return true
			}
		case *net.IPAddr:
			if v.IP.To4() != nil && v.IP.Equal(want) {
				return true
			}
		}
	}
	return false
}

// FindAdapterIfIndex 按 TUN 配置名解析 ifIndex（与 InterfaceIndex 同义）。
//
// CODEMAP 推荐此名表达「找网卡」；实现委托 InterfaceIndex（LUID→ByName→PS）。
func FindAdapterIfIndex(name string) (int, error) {
	return InterfaceIndex(name)
}

// findAdapterIfIndexPS 通过 PowerShell 按 Name 或 Wintun/品牌池描述查找网卡 ifIndex。
//
// 找网卡模板唯一源：PSSnippetAssignAdapterIf。
func findAdapterIfIndexPS(name string) (int, error) {
	ps := PSSnippetAssignAdapterIf(name) + `
if (-not $if) { throw 'adapter not found' }
Write-Output $if.ifIndex
`
	out, err := RunPSOneShot(ps)
	if err != nil {
		return 0, fmt.Errorf("查网卡索引 %q: %w", name, err)
	}
	idx, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || idx <= 0 {
		return 0, fmt.Errorf("无效 ifIndex=%q for %s", strings.TrimSpace(string(out)), name)
	}
	return idx, nil
}
