package probedefense_test

import (
	"path/filepath"
	"testing"
	"time"

	"haovpn/internal/persist"
	"haovpn/internal/probedefense"
)

// TestManualBanDurationPermanent 手动封禁 durationSec=0 时 expires_at 为空（永久）。
func TestManualBanDurationPermanent(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "manual_perm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	g := probedefense.New(store, probedefense.DefaultConfig())
	if err := g.ManualBan("203.0.113.10", "永久测试", 0); err != nil {
		t.Fatal(err)
	}
	b, err := store.GetActiveIPBlock("203.0.113.10")
	if err != nil || b == nil {
		t.Fatal("expected active block")
	}
	if b.ExpiresAt != nil {
		t.Fatalf("permanent ban should have nil expires_at, got %v", b.ExpiresAt)
	}
}

// TestManualBanDurationOneWeek 手动封禁指定 1 周时长。
func TestManualBanDurationOneWeek(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "manual_week.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const week = 7 * 24 * 3600
	before := time.Now()
	g := probedefense.New(store, probedefense.DefaultConfig())
	if err := g.ManualBan("203.0.113.11", "一周测试", week); err != nil {
		t.Fatal(err)
	}
	b, err := store.GetActiveIPBlock("203.0.113.11")
	if err != nil || b == nil || b.ExpiresAt == nil {
		t.Fatal("expected block with expiry")
	}
	wantMin := before.Add(time.Duration(week-5) * time.Second)
	wantMax := time.Now().Add(time.Duration(week+5) * time.Second)
	if b.ExpiresAt.Before(wantMin) || b.ExpiresAt.After(wantMax) {
		t.Fatalf("expires_at out of range: got %v want ~%ds from now", b.ExpiresAt, week)
	}
}

// TestManualBanDurationDefault 省略 duration 时使用 cfg.BanDurationSec。
func TestManualBanDurationDefault(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "manual_def.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := probedefense.DefaultConfig()
	cfg.BanDurationSec = 7200
	before := time.Now()
	g := probedefense.New(store, cfg)
	if err := g.ManualBan("203.0.113.12", "默认时长", probedefense.BanDurationUseDefault); err != nil {
		t.Fatal(err)
	}
	b, err := store.GetActiveIPBlock("203.0.113.12")
	if err != nil || b == nil || b.ExpiresAt == nil {
		t.Fatal("expected block with expiry from config default")
	}
	wantMin := before.Add(7195 * time.Second)
	wantMax := time.Now().Add(7205 * time.Second)
	if b.ExpiresAt.Before(wantMin) || b.ExpiresAt.After(wantMax) {
		t.Fatalf("expires_at out of range: got %v want ~7200s", b.ExpiresAt)
	}
}

// TestValidateManualBanDuration 边界校验。
func TestValidateManualBanDuration(t *testing.T) {
	if err := probedefense.ValidateManualBanDuration(probedefense.BanDurationUseDefault); err != nil {
		t.Fatal(err)
	}
	if err := probedefense.ValidateManualBanDuration(0); err != nil {
		t.Fatal(err)
	}
	if err := probedefense.ValidateManualBanDuration(3600); err != nil {
		t.Fatal(err)
	}
	if err := probedefense.ValidateManualBanDuration(-2); err == nil {
		t.Fatal("negative except UseDefault should fail")
	}
	if err := probedefense.ValidateManualBanDuration(30); err == nil {
		t.Fatal("too short should fail")
	}
	if err := probedefense.ValidateManualBanDuration(probedefense.MaxBanDurationSec + 1); err == nil {
		t.Fatal("over max should fail")
	}
}
