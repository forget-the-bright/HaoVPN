package netstack_test

import (
	"testing"

	"haovpn/internal/netstack"
)

// TestICSLifecyclePreserve 钉死 Disable/Preserve 语义，避免 Logout 误用 Preserve。
func TestICSLifecyclePreserve(t *testing.T) {
	if netstack.ICSDisable.Preserve() {
		t.Fatal("ICSDisable 不得 Preserve")
	}
	if !netstack.ICSPreserve.Preserve() {
		t.Fatal("ICSPreserve 须 Preserve")
	}
	if netstack.ICSDisable.LogLabel() != "disable" || netstack.ICSPreserve.LogLabel() != "preserve" {
		t.Fatalf("LogLabel 不符: %q %q", netstack.ICSDisable.LogLabel(), netstack.ICSPreserve.LogLabel())
	}
}
