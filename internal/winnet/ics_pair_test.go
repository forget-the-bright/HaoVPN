package winnet

import "testing"

func TestICSPairRememberTake(t *testing.T) {
	ClearICSPair()
	defer ClearICSPair()
	if _, _, ok := TakeICSPair(); ok {
		t.Fatal("空时应 false")
	}
	RememberICSPair("WLAN", "haovpn_client")
	pub, prv, ok := TakeICSPair()
	if !ok || pub != "WLAN" || prv != "haovpn_client" {
		t.Fatalf("got %q %q ok=%v", pub, prv, ok)
	}
	if _, _, ok := TakeICSPair(); ok {
		t.Fatal("Take 后应清空")
	}
}
