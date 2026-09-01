package netutil

import "strings"

// ParseLocalLANsField 解析 GUI/配置中的本地网段文本（多行或逗号分隔）。
func ParseLocalLANsField(s string) []string {
	s = strings.ReplaceAll(s, ",", "\n")
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
