package probedefense

import (
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/timeutil"
)

// maybeAutoBan 窗口内 rejected 事件达阈值时自动写入 ip_blocks。
func (g *Guard) maybeAutoBan(ip, signature string) {
	if !g.cfg.AutoBan || ip == "" {
		return
	}
	if g.IsAllowlisted(ip) || g.IsBanExempt(ip) {
		return
	}
	for _, ign := range g.cfg.IgnoreSignaturesForBan {
		if signature == strings.TrimSpace(ign) {
			return
		}
	}
	window := timeutil.Seconds(g.cfg.BanWindowSec)
	if window <= 0 {
		window = 10 * time.Minute
	}
	threshold := g.cfg.BanAfterEvents
	if threshold <= 0 {
		threshold = 8
	}
	n, err := g.store.CountSecurityEventsSince(ip, time.Now().Add(-window), g.cfg.IgnoreSignaturesForBan)
	if err != nil || n < threshold {
		return
	}
	reason := "auto: " + signature + " 窗口内达阈值"
	b := persist.IPBlock{
		IP: ip, Reason: reason, Source: "auto", Signature: signature, Enabled: true,
	}
	b.ExpiresAt = g.resolveBanExpiry(BanDurationUseDefault)
	if err := g.store.UpsertIPBlock(b); err != nil {
		logger.Warn("自动封禁失败 ip=%s: %v", ip, err)
		return
	}
	g.record(ip, "", PhaseTCPAccept, signature, ActionAutoBanned, reason)
	logger.Warn("探针防御自动封禁 ip=%s reason=%s events=%d", ip, reason, n)
}
