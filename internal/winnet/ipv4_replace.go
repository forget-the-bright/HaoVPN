package winnet

import (
	"fmt"
	"net"
)

// UnicastIPv4Entry 网卡上一条 IPv4 单播（含前缀与 SkipAsSource，供替换式删除/PreferVPN）。
type UnicastIPv4Entry struct {
	IP           net.IP
	PrefixLen    int
	SkipAsSource bool
}

// FormatUnicastIPv4Entries 将条目格式化为日志友好切片（如 "10.88.0.2/24"）。
func FormatUnicastIPv4Entries(ents []UnicastIPv4Entry) []string {
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		if e.IP == nil {
			continue
		}
		out = append(out, fmt.Sprintf("%s/%d", e.IP.String(), e.PrefixLen))
	}
	return out
}
