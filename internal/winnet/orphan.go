package winnet

import (
	"strings"

	"haovpn/internal/brand"
)

// IsWintunOrphanAdapterName 判定友好名是否为 configName 的孤儿后缀网卡（如 haovpn_client 1）。
//
// 本函数是孤儿名判定的**唯一导出 API**（曾另有无调用方的 IsWintunOrphanName，已删，避免双规则漂移）。
// 规则与 BuildPrepareWintunOrphanScript 对齐意图：Name≠want 且 Name 以 want 为前缀且后缀非字母。
// desc 含 Wintun 或品牌池时更确信；desc 空时仅按名称判定（探测方可能无描述）。
// 关联：HasWintunOrphanAdapters（原生枚举）；PS 清理脚本见 BuildPrepareWintunOrphanScript。
func IsWintunOrphanAdapterName(configName, friendlyName, description string) bool {
	want := strings.TrimSpace(configName)
	name := strings.TrimSpace(friendlyName)
	if want == "" || name == "" || strings.EqualFold(name, want) {
		return false
	}
	// 前缀匹配：haovpn0 1 / haovpn0#2 等
	if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(want)) {
		return false
	}
	// 紧跟非字母数字则视为后缀孤儿（空格、#、数字）
	rest := name[len(want):]
	if rest == "" {
		return false
	}
	c := rest[0]
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
		return false // 避免 haovpn0extra 误伤
	}
	desc := strings.ToLower(description)
	if desc == "" {
		return true
	}
	return strings.Contains(desc, "wintun") || strings.Contains(desc, strings.ToLower(brand.WintunPool))
}
