package probedefense

import (
	"fmt"
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
)

// ImportBanExemptFromYAML 将 server.yaml 中的 ban_exempt_ips 幂等导入 DB。
//
// 非法 IP/CIDR 跳过并打 WARN（宽容导入，与 config 其它列表一致）。
func ImportBanExemptFromYAML(store *persist.Store, ips []string) error {
	if store == nil {
		return nil
	}
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if err := netutil.ValidateIPOrCIDR("ip", ip, true); err != nil {
			logger.Warn("跳过无效封禁豁免 %s: %v", ip, err)
			continue
		}
		if err := store.UpsertBanExempt(ip, "来自 server.yaml", "yaml_import"); err != nil {
			return fmt.Errorf("导入封禁豁免 %s: %w", ip, err)
		}
	}
	return nil
}

// ReloadBanExempt 从 DB 重新加载生效豁免列表并与 yaml 静态配置合并（去重）。
func (g *Guard) ReloadBanExempt() error {
	if g == nil || g.store == nil {
		return nil
	}
	fromDB, err := g.store.ListEnabledBanExemptIPs()
	if err != nil {
		return err
	}
	merged := netutil.MergeDedupTrimNonEmpty(g.cfg.BanExemptIPs, fromDB)
	g.exemptMu.Lock()
	g.banExemptIPs = merged
	g.exemptMu.Unlock()
	logger.Info("封禁豁免列表已加载 count=%d", len(merged))
	return nil
}

// SetBanExemptIPs 热更新内存豁免列表（API 写库后调用 ReloadBanExempt 即可，本函数供测试）。
func (g *Guard) SetBanExemptIPs(ips []string) {
	if g == nil {
		return
	}
	g.exemptMu.Lock()
	g.banExemptIPs = append([]string(nil), ips...)
	g.exemptMu.Unlock()
}

// IsBanExempt 是否处于封禁豁免名单（永不封禁、不受 ip_blocks 影响）。
func (g *Guard) IsBanExempt(ip string) bool {
	if g == nil || ip == "" {
		return false
	}
	parsed, err := netutil.ParseHostIP(ip)
	if err != nil {
		return false
	}
	g.exemptMu.RLock()
	rules := g.banExemptIPs
	g.exemptMu.RUnlock()
	if len(rules) == 0 {
		return false
	}
	return netutil.IPMatchesRules(parsed, rules)
}

