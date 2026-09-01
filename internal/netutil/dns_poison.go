package netutil

import "strings"

// DNSServersPoisoned 判断 DNS 快照是否被旧/新 VPN IP 等污染。
func DNSServersPoisoned(servers, poison []string) bool {
	if len(servers) == 0 || len(poison) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(poison))
	for _, p := range poison {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		set[p] = struct{}{}
	}
	if len(set) == 0 {
		return false
	}
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if _, ok := set[s]; ok {
			return true
		}
	}
	return false
}

// FilterDNSServersPoison 从 DNS 列表去掉 poison 中的地址（纵深过滤）。
func FilterDNSServersPoison(servers, poison []string) []string {
	if len(servers) == 0 {
		return nil
	}
	if len(poison) == 0 {
		out := make([]string, len(servers))
		copy(out, servers)
		return out
	}
	set := make(map[string]struct{}, len(poison))
	for _, p := range poison {
		p = strings.TrimSpace(p)
		if p != "" {
			set[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(servers))
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, bad := set[s]; bad {
			continue
		}
		out = append(out, s)
	}
	return out
}
