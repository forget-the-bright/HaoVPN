package probedefense_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"haovpn/internal/persist"
	"haovpn/internal/probedefense"
)

// TestRecordRejectEnabledRecordEventsMatrix enabled/record_events 组合：落库与 auto-ban 解耦。
func TestRecordRejectEnabledRecordEventsMatrix(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "rec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cases := []struct {
		name         string
		enabled      bool
		recordEvents bool
		wantEvents   int
	}{
		{"both_on", true, true, 1},
		{"record_only", false, true, 1},
		{"enabled_only", true, false, 0},
		{"both_off", false, false, 0},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ip := fmt.Sprintf("10.0.0.%d", i+1)
			cfg := probedefense.DefaultConfig()
			cfg.Enabled = tc.enabled
			cfg.RecordEvents = tc.recordEvents
			g := probedefense.New(store, cfg)
			g.RecordReject(ip, "9", probedefense.PhaseTLS, probedefense.SigTLSBadRecord, "test")
			n, _, err := store.ListSecurityEvents(persist.SecurityEventFilter{ClientIP: ip, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if len(n) != tc.wantEvents {
				t.Fatalf("events=%d want %d", len(n), tc.wantEvents)
			}
		})
	}
}
