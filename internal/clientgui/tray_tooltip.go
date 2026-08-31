package clientgui

import (
	"strings"
	"time"
	"unicode/utf16"

	"haovpn/internal/brand"
	"haovpn/internal/clientapp"
)

// windowsTrayTipMaxUTF16 托盘悬停可见 UTF-16 上限（含实务 NUL 预留后的内容长度）。
//
// fyne.io/systray 未调用 NIM_SETVERSION/NOTIFYICON_VERSION_4 时，Windows 只展示约 64 码元；
// 按 127 拼装会被系统砍断（如「连接自: 2026/…」残片成「连接自: 20」）。产品以 63 为准。
const windowsTrayTipMaxUTF16 = 63

// trayTipPhase 托盘 tip 展示相位（可覆盖 Engine.State，如正在断开）。
type trayTipPhase int

const (
	trayTipFromEngine trayTipPhase = iota
	trayTipDisconnecting
)

// trayTooltipInput 组装悬停文案所需字段（纯数据，便于单测）。
type trayTooltipInput struct {
	State     clientapp.State
	Phase     trayTipPhase // Disconnecting 优先于 State
	Server    string
	VPNIP     string
	Since     time.Time
	LastError string
}

// formatTrayTooltip 生成托盘悬停文案：IP 优先、按 UTF-16 预算整行拼装，禁止半截日期。
func formatTrayTooltip(in trayTooltipInput) string {
	if in.Phase == trayTipDisconnecting {
		return fitTipLines(brand.Name, "正在断开…")
	}
	switch in.State {
	case clientapp.StateConnected:
		return formatConnectedTip(in)
	case clientapp.StateConnecting:
		ip := strings.TrimSpace(in.VPNIP)
		if ip != "" {
			return fitTipLines(brand.Name, "正在配置网络…", "分配 IP: "+ip)
		}
		return fitTipLines(brand.Name, "正在连接…")
	case clientapp.StateReconnecting:
		return fitTipLines(brand.Name, "重连中…")
	default:
		errMsg := strings.TrimSpace(in.LastError)
		if errMsg != "" && in.State == clientapp.StateIdle {
			return fitTipLines(brand.Name, "错误: "+truncateUTF16(errMsg, 40))
		}
		return fitTipLines(brand.Name, "未连接")
	}
}

// formatConnectedTip 行序：品牌 → 分配 IP → 连接自 → 已连接至。
// 放不下则整行丢主机或整行丢 since，禁止「连接自: 20」这类半截。
func formatConnectedTip(in trayTooltipInput) string {
	ip := strings.TrimSpace(in.VPNIP)
	if ip == "" {
		ip = "—"
	}
	ipLine := "分配 IP: " + ip
	base := brand.Name + "\n" + ipLine
	if utf16Len(base) > windowsTrayTipMaxUTF16 {
		return truncateUTF16(base, windowsTrayTipMaxUTF16)
	}

	sinceLine := ""
	if !in.Since.IsZero() {
		// 短日期省预算（如 8/31 19:51）
		sinceLine = "连接自: " + in.Since.Format("1/2 15:04")
	}

	server := strings.TrimSpace(in.Server)
	serverLine := ""
	if server != "" {
		serverLine = "已连接至: " + server
	}

	// 先尝试：base + since + server（主机可 ellipsize）
	if sinceLine != "" {
		withSince := base + "\n" + sinceLine
		if utf16Len(withSince) > windowsTrayTipMaxUTF16 {
			return base // since 整行放不下则不要半截
		}
		if serverLine == "" {
			return withSince
		}
		remain := windowsTrayTipMaxUTF16 - utf16Len(withSince) - 1 // \n
		prefix := "已连接至: "
		hostBudget := remain - utf16Len(prefix)
		if hostBudget < 4 {
			return withSince // 整行丢主机
		}
		host := ellipsizeUTF16(server, hostBudget)
		out := withSince + "\n" + prefix + host
		if utf16Len(out) > windowsTrayTipMaxUTF16 {
			return withSince
		}
		return out
	}

	// 无 since：尽量加主机
	if serverLine == "" {
		return base
	}
	remain := windowsTrayTipMaxUTF16 - utf16Len(base) - 1
	prefix := "已连接至: "
	hostBudget := remain - utf16Len(prefix)
	if hostBudget < 4 {
		return base
	}
	out := base + "\n" + prefix + ellipsizeUTF16(server, hostBudget)
	if utf16Len(out) > windowsTrayTipMaxUTF16 {
		return base
	}
	return out
}

// fitTipLines 用换行拼接多行并保证总长 ≤ tip 上限（后行整行可丢，不砍半行）。
func fitTipLines(lines ...string) string {
	var parts []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cand := line
		if len(parts) > 0 {
			cand = strings.Join(parts, "\n") + "\n" + line
		}
		if utf16Len(cand) > windowsTrayTipMaxUTF16 {
			if len(parts) == 0 {
				return truncateUTF16(line, windowsTrayTipMaxUTF16)
			}
			break
		}
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		return brand.Name
	}
	return strings.Join(parts, "\n")
}

func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// ellipsizeUTF16 中间省略，保证 UTF-16 长度 ≤ maxUnits。
func ellipsizeUTF16(s string, maxUnits int) string {
	if maxUnits <= 0 {
		return ""
	}
	if utf16Len(s) <= maxUnits {
		return s
	}
	if maxUnits <= 1 {
		return truncateUTF16(s, maxUnits)
	}
	ellipsis := "…"
	inner := maxUnits - utf16Len(ellipsis)
	if inner < 2 {
		return truncateUTF16(s, maxUnits)
	}
	left := inner / 2
	right := inner - left
	runes := []rune(s)
	if left > len(runes) {
		left = len(runes)
	}
	head := string(runes[:left])
	for utf16Len(head) > left && len([]rune(head)) > 0 {
		r := []rune(head)
		head = string(r[:len(r)-1])
	}
	tailRunes := runes
	if right < len(tailRunes) {
		tailRunes = runes[len(runes)-right:]
	}
	tail := string(tailRunes)
	for utf16Len(head)+utf16Len(ellipsis)+utf16Len(tail) > maxUnits && len([]rune(tail)) > 0 {
		tr := []rune(tail)
		tail = string(tr[1:])
	}
	return head + ellipsis + tail
}

// truncateUTF16 按 UTF-16 码元截断，尽量在完整 rune 边界切断。
func truncateUTF16(s string, maxUnits int) string {
	if maxUnits <= 0 {
		return ""
	}
	u := utf16.Encode([]rune(s))
	if len(u) <= maxUnits {
		return s
	}
	u = u[:maxUnits]
	if len(u) > 0 {
		last := u[len(u)-1]
		if last >= 0xD800 && last <= 0xDBFF {
			u = u[:len(u)-1]
		}
	}
	return string(utf16.Decode(u))
}
