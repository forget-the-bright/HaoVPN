package winnet

import (
	"strings"

	"haovpn/internal/logger"
)

// ICSPSLogInfo ICS/Prefer 相关 PowerShell stdout 的解析结果。
type ICSPSLogInfo struct {
	// SawSharedAccessRestart 是否见到 ics_sharedaccess action=restart（冷启 Force Restart 验收）。
	SawSharedAccessRestart bool
}

// LogICSPowerShellLines 把 ICS Enable / Prefer 脚本 stdout 中的关键行打到 Info。
//
// 识别前缀：ics_stage / ics_sharedaccess / ics_src_diag / ics_prefix_keep /
// ics_default_route_scrubbed / ics_prefer_vpn。
// 不再识别 ics_prefix_fix（已禁止；若旧脚本误发也不当成功信号）。
//
// 为何集中：enableICSCold、PreferVPNSourceWithICSContext、prefer_vpn_light 曾复制三段 switch。
func LogICSPowerShellLines(out []byte) ICSPSLogInfo {
	var info ICSPSLogInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "ics_stage "),
			strings.HasPrefix(line, "ics_sharedaccess "),
			strings.HasPrefix(line, "ics_src_diag "),
			strings.HasPrefix(line, "ics_prefix_keep "),
			strings.HasPrefix(line, "ics_default_route_scrubbed "),
			strings.HasPrefix(line, "ics_prefer_vpn "):
			logger.Info("windows: %s", line)
			if strings.HasPrefix(line, "ics_sharedaccess ") && strings.Contains(line, "action=restart") {
				info.SawSharedAccessRestart = true
			}
		}
	}
	return info
}
