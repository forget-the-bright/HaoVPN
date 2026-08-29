package clientapp_test

import (
	"testing"

	"haovpn/internal/persist"
)

// TestValidLANCIDRsEmptyMeansOff 空/无效列表视为未开启 via。
func TestValidLANCIDRsEmptyMeansOff(t *testing.T) {
	if got := persist.ValidLANCIDRs(nil); len(got) != 0 {
		t.Fatalf("nil -> %v", got)
	}
	if got := persist.ValidLANCIDRs([]string{"", "0.0.0.0/0", "bad"}); len(got) != 0 {
		t.Fatalf("invalid -> %v", got)
	}
	got := persist.ValidLANCIDRs([]string{"192.168.31.0/24", "192.168.31.0/24"})
	if len(got) != 1 || got[0] != "192.168.31.0/24" {
		t.Fatalf("got %v", got)
	}
}
