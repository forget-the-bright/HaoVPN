package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"haovpn/internal/config"
	"gopkg.in/yaml.v3"
)

// TestProbeDefenseEnabledFalseNotReopened 仅写 enabled: false 时 ApplyDefaults 不得改回 true。
func TestProbeDefenseEnabledFalseNotReopened(t *testing.T) {
	var cfg config.ServerConfig
	raw := []byte(`
security:
  probe_defense:
    enabled: false
`)
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.ApplyDefaults()
	if cfg.Security.ProbeDefense.IsEnabled() {
		t.Fatal("enabled: false 被 ApplyDefaults 改回 true")
	}
	if !cfg.Security.ProbeDefense.IsRecordEvents() || !cfg.Security.ProbeDefense.IsAutoBan() {
		t.Fatal("未写的 record_events/auto_ban 应变为默认 true")
	}
}

// TestProbeDefenseFreshDefaultsOn 整段未配置时默认开启并填封禁时长。
func TestProbeDefenseFreshDefaultsOn(t *testing.T) {
	var cfg config.ServerConfig
	cfg.ApplyDefaults()
	pd := cfg.Security.ProbeDefense
	if !pd.IsEnabled() || !pd.IsRecordEvents() || !pd.IsAutoBan() {
		t.Fatal("未配置应默认开启")
	}
	if pd.BanDurationSec != 3600 {
		t.Fatalf("BanDurationSec=%d want 3600", pd.BanDurationSec)
	}
	if pd.BanAfterEvents != 8 || pd.BanWindowSec != 600 {
		t.Fatalf("阈值默认错误 after=%d window=%d", pd.BanAfterEvents, pd.BanWindowSec)
	}
}

// TestProbeDefenseBanDurationZeroPermanent 显式 ban_duration_sec:0 表示永久，不被改成 3600。
func TestProbeDefenseBanDurationZeroPermanent(t *testing.T) {
	var cfg config.ServerConfig
	raw := []byte(`
security:
  probe_defense:
    enabled: true
    ban_duration_sec: 0
`)
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.ApplyDefaults()
	if cfg.Security.ProbeDefense.BanDurationSec != 0 {
		t.Fatalf("显式 0 应表示永久，got %d", cfg.Security.ProbeDefense.BanDurationSec)
	}
}

// TestLoadServerProbeDefenseRoundTrip 默认生成的 yaml 含 probe_defense 且加载后仍开启。
func TestLoadServerProbeDefenseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	cfg, _, err := config.LoadServer(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Security.ProbeDefense.IsEnabled() {
		t.Fatal("默认模板应启用探针防御")
	}
	data, _ := os.ReadFile(path)
	if !contains(string(data), "probe_defense") {
		t.Fatal("默认 yaml 应含 probe_defense")
	}
}
