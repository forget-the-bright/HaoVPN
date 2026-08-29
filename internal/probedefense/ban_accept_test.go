package probedefense_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/persist"
	"haovpn/internal/probedefense"
)

// TestAllowAcceptHonorsBanWhenDisabled 封禁表在 Enabled=false 时仍须拒绝 Accept（与 hardening 文档一致）。
func TestAllowAcceptHonorsBanWhenDisabled(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "ban.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := probedefense.DefaultConfig()
	cfg.Enabled = false
	g := probedefense.New(store, cfg)
	if err := g.ManualBan("203.0.113.9", "test manual"); err != nil {
		t.Fatal(err)
	}
	if g.AllowAccept("203.0.113.9:4444") {
		t.Fatal("banned IP must be rejected even when Enabled=false")
	}
	if !g.AllowAccept("198.51.100.1:1") {
		t.Fatal("unbanned IP should pass when Enabled=false")
	}
}
