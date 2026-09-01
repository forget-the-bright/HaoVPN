package winnet_test

import (
	"testing"

	"haovpn/internal/netutil"
)

func TestPreferSkipAsSourceNeedsUpdate(t *testing.T) {
	if netutil.PreferSkipAsSourceNeedsUpdate(false, true, true) {
		t.Fatal("已正确时不应更新")
	}
	if !netutil.PreferSkipAsSourceNeedsUpdate(true, true, true) {
		t.Fatal("vpn skip=true 应更新")
	}
	if !netutil.PreferSkipAsSourceNeedsUpdate(false, true, false) {
		t.Fatal("137 skip=false 应更新")
	}
	if netutil.PreferSkipAsSourceNeedsUpdate(false, false, false) {
		t.Fatal("无 137 且 vpn 正确时不应更新")
	}
}
