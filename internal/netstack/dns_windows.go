//go:build windows

package netstack

import (
	"fmt"
	"strings"
	"sync"

	"haovpn/internal/platform"
)

// dnsState 记录适配器原 DNS 以便恢复。
type dnsState struct {
	dhcp    bool
	servers []string
}

var (
	dnsMu    sync.Mutex
	dnsSaved = map[string]dnsState{}
)

// ApplyDNS 在 TUN 适配器上设置静态 DNS；首次调用时快照原 DNS。
func ApplyDNS(adapterName string, servers []string) error {
	if adapterName == "" || len(servers) == 0 {
		return nil
	}
	dnsMu.Lock()
	defer dnsMu.Unlock()
	if _, ok := dnsSaved[adapterName]; !ok {
		dhcp, prior, _ := readDNS(adapterName)
		dnsSaved[adapterName] = dnsState{dhcp: dhcp, servers: append([]string{}, prior...)}
	}
	return applyStaticDNS(adapterName, servers)
}

// RestoreDNS 按快照恢复；不经 ApplyDNS，避免把当前 VPN DNS 存成「原 DNS」。
func RestoreDNS(adapterName string) error {
	if adapterName == "" {
		return nil
	}
	dnsMu.Lock()
	st, ok := dnsSaved[adapterName]
	if ok {
		delete(dnsSaved, adapterName)
	}
	dnsMu.Unlock()
	if !ok {
		return nil
	}
	if st.dhcp || len(st.servers) == 0 {
		out, err := platform.Command("netsh", "interface", "ipv4", "set", "dnsservers", adapterName, "source=dhcp").CombinedOutput()
		if err != nil {
			return fmt.Errorf("netsh dhcp dns: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return applyStaticDNS(adapterName, st.servers)
}

// applyStaticDNS 只写系统 DNS，不读写 dnsSaved。
func applyStaticDNS(adapterName string, servers []string) error {
	if len(servers) == 0 {
		return nil
	}
	args := []string{"interface", "ipv4", "set", "dnsservers", adapterName, "source=static", "address=" + servers[0], "register=none", "validate=no"}
	if out, err := platform.Command("netsh", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("netsh set dns: %w: %s", err, strings.TrimSpace(string(out)))
	}
	for i := 1; i < len(servers); i++ {
		args := []string{"interface", "ipv4", "add", "dnsservers", adapterName, servers[i], "index=" + fmt.Sprintf("%d", i+1), "validate=no"}
		if out, err := platform.Command("netsh", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("netsh add dns: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func readDNS(adapterName string) (dhcp bool, servers []string, err error) {
	out, err := platform.Command("netsh", "interface", "ipv4", "show", "dnsservers", adapterName).CombinedOutput()
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

// DNSSavedCount 返回当前快照条目数（单测用）。
func DNSSavedCount() int {
	dnsMu.Lock()
	defer dnsMu.Unlock()
	return len(dnsSaved)
}

// ClearDNSSavedForTest 清空快照（单测用）。
func ClearDNSSavedForTest() {
	dnsMu.Lock()
	dnsSaved = map[string]dnsState{}
	dnsMu.Unlock()
}

// NoteSavedDNSForTest 注入快照（单测用）。
func NoteSavedDNSForTest(adapter string, dhcp bool, servers []string) {
	dnsMu.Lock()
	dnsSaved[adapter] = dnsState{dhcp: dhcp, servers: append([]string{}, servers...)}
	dnsMu.Unlock()
}

// TakeDNSSavedForTest 取出并删除快照（模拟 Restore 的存档逻辑，单测用）。
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
