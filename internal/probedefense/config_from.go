package probedefense

import "haovpn/internal/config"

// ConfigFromServer 从服务端 Security 段构造 Guard 配置。
//
// 调用前须已 ApplyDefaults，使 *bool 已解为非 nil。
func ConfigFromServer(sec config.SecuritySection) Config {
	pd := sec.ProbeDefense
	return Config{
		Enabled:                pd.IsEnabled(),
		RecordEvents:           pd.IsRecordEvents(),
		AutoBan:                pd.IsAutoBan(),
		BanAfterEvents:         pd.BanAfterEvents,
		BanWindowSec:           pd.BanWindowSec,
		BanDurationSec:         pd.BanDurationSec,
		EventRetentionDays:     pd.EventRetentionDays,
		IgnoreSignaturesForBan: pd.IgnoreSignaturesForBan,
		AllowedSourceIPs:       sec.TunnelAllowedSourceIPs,
	}
}
