package winnet_test

import (
	"testing"

	"haovpn/internal/winnet"
)

// TestInterfaceHasICSPrivateBadIndex 非法 ifIndex 恒 false（不调系统表）。
func TestInterfaceHasICSPrivateBadIndex(t *testing.T) {
	if winnet.InterfaceHasICSPrivate(0) || winnet.InterfaceHasICSPrivate(-1) {
		t.Fatal("ifIndex<=0 须 false")
	}
}
