//go:build windows

package netstack

import (
	"fmt"
	"strings"
	"sync"

	"haovpn/internal/winnet"
)

// dnsState 记录适配器原 DNS 以便恢复。
type dnsState struct {
	dhcp    bool
	servers []string
}

var (
	dnsMu      sync.Mutex
	dnsSaved   = map[string]dnsState{}
	dnsApplied = map[string][]string{} // 当前会话已写入系统的 DNS（避免重连重复 netsh）
)

// ApplyDNS 在 TUN 适配器上写入静态 DNS 服务器列表。
//
// 参数：
//   adapterName — TUN 配置名；经 winnet.ResolveInterfaceAlias 转为 netsh 别名。
//   servers — 握手推送的 DNS IPv4 列表；空则直接返回 nil。
// 返回：netsh 失败时 error。
// 副作用：首次调用快照原 DHCP/静态 DNS 至内存；重复相同列表跳过 netsh。
// 并发：包内 dnsMu 串行化；可重入但不宜并行对不同适配器高频调用。
func ApplyDNS(adapterName string, servers []string) error {
	if adapterName == "" || len(servers) == 0 {
		return nil
	}
	resolved := winnet.ResolveInterfaceAlias(adapterName)
	dnsMu.Lock()
	defer dnsMu.Unlock()
	if prior, ok := dnsApplied[resolved]; ok && dnsServersEqual(prior, servers) {
		return nil
	}
	if _, ok := dnsSaved[resolved]; !ok {
		dhcp, prior, _ := readDNS(resolved)
		dnsSaved[resolved] = dnsState{dhcp: dhcp, servers: append([]string{}, prior...)}
	}
	if err := applyStaticDNS(resolved, servers); err != nil {
		return err
	}
	dnsApplied[resolved] = append([]string{}, servers...)
	return nil
}

// RestoreDNS 断开或 Teardown 时按快照恢复 TUN 适配器 DNS。
//
// 参数：adapterName — 与 ApplyDNS 相同的 TUN 配置名。
// 返回：netsh 恢复 DHCP/静态 DNS 失败时 error；无快照时 nil。
// 副作用：清除 dnsSaved/dnsApplied 中该适配器条目；不经过 ApplyDNS 以免污染快照。
func RestoreDNS(adapterName string) error {
	if adapterName == "" {
		return nil
	}
	resolved := winnet.ResolveInterfaceAlias(adapterName)
	dnsMu.Lock()
	st, ok := dnsSaved[resolved]
	if ok {
		delete(dnsSaved, resolved)
	}
	delete(dnsApplied, resolved)
	dnsMu.Unlock()
	if !ok {
		return nil
	}
	if st.dhcp || len(st.servers) == 0 {
		if err := winnet.RestoreInterfaceDNSDHCP(resolved); err != nil {
			return fmt.Errorf("netsh dhcp dns: %w", err)
		}
		return nil
	}
	return applyStaticDNS(resolved, st.servers)
}

func dnsServersEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func applyStaticDNS(adapterName string, servers []string) error {
	if len(servers) == 0 {
		return nil
	}
	if err := winnet.SetInterfaceDNSStatic(adapterName, servers[0]); err != nil {
		return err
	}
	for i := 1; i < len(servers); i++ {
		if err := winnet.AddInterfaceDNS(adapterName, servers[i], i+1); err != nil {
			return err
		}
	}
	return nil
}

func readDNS(adapterName string) (dhcp bool, servers []string, err error) {
	out, err := winnet.ShowInterfaceDNS(adapterName)
	if err != nil {
		return true, nil, err
	}
	text := string(out)
	if strings.Contains(text, "DNS servers configured through DHCP") ||
		(strings.Contains(text, "DHCP") && strings.Contains(text, "DNS")) {
		return true, nil, nil
	}
	return false, ParseDNSShowOutput(out), nil
}

// DNSSavedCount 返回当前尚未 Restore 的 DNS 快照条目数。
//
// 用途：单测断言 ApplyDNS/RestoreDNS 配对是否正确。
func DNSSavedCount() int {
	dnsMu.Lock()
	defer dnsMu.Unlock()
	return len(dnsSaved)
}

// ClearDNSSavedForTest 清空 dnsSaved 与 dnsApplied 全局状态。
//
// 用途：单测 beforeEach 隔离；生产代码勿调用。
func ClearDNSSavedForTest() {
	dnsMu.Lock()
	dnsSaved = map[string]dnsState{}
	dnsApplied = map[string][]string{}
	dnsMu.Unlock()
}

// NoteSavedDNSForTest 向 dnsSaved 注入伪造快照，模拟曾调用 ApplyDNS。
//
// 参数：adapter — 适配器别名；dhcp — 原是否为 DHCP；servers — 原静态 DNS 列表。
func NoteSavedDNSForTest(adapter string, dhcp bool, servers []string) {
	dnsMu.Lock()
	dnsSaved[adapter] = dnsState{dhcp: dhcp, servers: append([]string{}, servers...)}
	dnsMu.Unlock()
}

// TakeDNSSavedForTest 取出并删除指定适配器的 DNS 快照（与 RestoreDNS 读档逻辑一致）。
//
// 返回：ok 为 false 表示无快照；servers 为快照副本，调用方可安全修改。
func TakeDNSSavedForTest(adapter string) (dhcp bool, servers []string, ok bool) {
	dnsMu.Lock()
	defer dnsMu.Unlock()
	st, ok := dnsSaved[adapter]
	if !ok {
		return false, nil, false
	}
	delete(dnsSaved, adapter)
	return st.dhcp, append([]string{}, st.servers...), true
}
