package probedefense

import (
	"strings"
	"time"

	"haovpn/internal/netutil"
	"haovpn/internal/persist"
	"haovpn/internal/timeutil"
)

// ManualBanStore 向 ip_blocks 写入手动封禁（须先过豁免检查）。
//
// 参数：
//   store — SQLite 存储；不可为 nil。
//   isExempt — 豁免判定；nil 表示不检查（仅单元测试）。
//   ip — 单 IP（非 CIDR）；须通过 ValidateIPOrCIDR。
//   reason — 封禁原因；空时由调用方填默认文案。
//   durationSec — BanDurationUseDefault(-1) 用 defaultBanSec；0 永久；>0 指定秒数。
//   defaultBanSec — durationSec 为 UseDefault 时的默认封禁秒数（通常 cfg.BanDurationSec）。
func ManualBanStore(store *persist.Store, isExempt func(string) bool, ip, reason string, durationSec, defaultBanSec int) error {
	if store == nil {
		return ErrProbeGuardNotReady
	}
	ip = strings.TrimSpace(ip)
	if err := netutil.ValidateIPOrCIDR("ip", ip, false); err != nil {
		return ErrInvalidBanIP
	}
	if isExempt != nil && isExempt(ip) {
		return ErrBanExempt
	}
	b := persist.IPBlock{
		IP: ip, Reason: reason, Source: "manual", Enabled: true,
	}
	sec := durationSec
	if sec == BanDurationUseDefault {
		sec = defaultBanSec
	}
	if sec > 0 {
		exp := time.Now().Add(timeutil.Seconds(sec))
		b.ExpiresAt = &exp
	}
	return store.UpsertIPBlock(b)
}

// resolveBanExpirySec 根据 durationSec 与默认秒数计算封禁时长；<=0 表示永久。
func resolveBanExpirySec(durationSec, defaultBanSec int) (sec int, permanent bool) {
	sec = durationSec
	if sec == BanDurationUseDefault {
		sec = defaultBanSec
	}
	if sec <= 0 {
		return 0, true
	}
	return sec, false
}

// resolveBanExpiryTime 根据 durationSec 计算过期时间点；nil 表示永久。
func resolveBanExpiryTime(durationSec, defaultBanSec int) *time.Time {
	sec, permanent := resolveBanExpirySec(durationSec, defaultBanSec)
	if permanent {
		return nil
	}
	exp := time.Now().Add(timeutil.Seconds(sec))
	return &exp
}
