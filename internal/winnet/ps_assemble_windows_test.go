//go:build windows

package winnet_test

import (
	"strings"
	"testing"

	"haovpn/internal/winnet"
)

func TestPSAssignAdapterAndPreferVPN(t *testing.T) {
	ps := winnet.PSAssignAdapterAndPreferVPN("haovpn0", "10.88.0.2", 0)
	for _, frag := range []string{"haovpn0", "10.88.0.2", "SkipAsSource", "$prvIdx = $if.ifIndex"} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("missing %q: %s", frag, ps)
		}
	}
}

func TestPSAssignAdapterAndSkipAsSourceOnly(t *testing.T) {
	ps := winnet.PSAssignAdapterAndSkipAsSourceOnly("10.88.0.2", 23)
	if !strings.Contains(ps, "10.88.0.2") || !strings.Contains(ps, "$prvIdx = 23") {
		t.Fatalf("bad assemble: %s", ps)
	}
	for _, absent := range []string{"Get-NetAdapter", "Remove-NetRoute", "PrefixLength -ne 32"} {
		if strings.Contains(ps, absent) {
			t.Fatalf("light path should not contain %q", absent)
		}
	}
}
