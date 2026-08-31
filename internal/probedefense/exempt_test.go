package probedefense_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/persist"
	"haovpn/internal/probedefense"
	"haovpn/internal/transport"
)

// TestImportBanExemptFromYAMLSkipsInvalid 非法 CIDR 跳过导入。
func TestImportBanExemptFromYAMLSkipsInvalid(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "import_exempt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := probedefense.ImportBanExemptFromYAML(store, []string{"203.0.113.1", "not-an-ip", "10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	ips, err := store.ListEnabledBanExemptIPs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 2 {
		t.Fatalf("expected 2 valid exempts, got %v", ips)
	}
}

// TestBanExemptSkipsManualBanAndBlock 豁免 IP 不可手动封禁且 IsBlocked 为 false。
func TestBanExemptSkipsManualBanAndBlock(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "exempt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const ip = "203.0.113.77"
	if err := store.UpsertBanExempt(ip, "test", "manual"); err != nil {
		t.Fatal(err)
	}
	cfg := probedefense.DefaultConfig()
	g := probedefense.New(store, cfg)
	if err := g.ReloadBanExempt(); err != nil {
		t.Fatal(err)
	}
	if !g.IsBanExempt(ip) {
		t.Fatal("expected exempt")
	}
	if err := g.ManualBan(ip, "should fail", 3600); err == nil {
		t.Fatal("ManualBan on exempt should fail")
	}
	if err := store.UpsertIPBlock(persist.IPBlock{
		IP: ip, Reason: "old", Source: "manual", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if g.IsBlocked(ip) {
		t.Fatal("exempt IP should not be blocked")
	}
	allow, banner := g.CheckAccept(ip + ":1234")
	if !allow || banner != "" {
		t.Fatalf("exempt should pass accept, got allow=%v banner=%q", allow, banner)
	}
}

// TestCheckAcceptBannedBanner 已封禁非豁免 IP 返回 HAOVPN:IP_BANNED banner。
func TestCheckAcceptBannedBanner(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "banbanner.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const ip = "203.0.113.78"
	g := probedefense.New(store, probedefense.DefaultConfig())
	if err := g.ManualBan(ip, "test", 3600); err != nil {
		t.Fatal(err)
	}
	allow, banner := g.CheckAccept(ip + ":1")
	if allow || banner != transport.BannerIPBanned {
		t.Fatalf("want banned banner, got allow=%v banner=%q", allow, banner)
	}
}
