package winnet_test

import (
	"testing"

	"haovpn/internal/winnet"
)

func TestParseDNSShowOutput(t *testing.T) {
	sample := []byte("Configuration for interface \"haovpn0\"\n  Statically Configured DNS Servers:    8.8.8.8\n    1.1.1.1")
	got := winnet.ParseDNSShowOutput(sample)
	if len(got) < 2 {
		t.Fatalf("got %v", got)
	}
	if got[0] != "8.8.8.8" || got[1] != "1.1.1.1" {
		t.Fatalf("order/content got %v", got)
	}
}

func TestParseDNSShowOutputEmpty(t *testing.T) {
	if got := winnet.ParseDNSShowOutput(nil); len(got) != 0 {
		t.Fatalf("nil got %v", got)
	}
	if got := winnet.ParseDNSShowOutput([]byte("no ips here")); len(got) != 0 {
		t.Fatalf("no-ip got %v", got)
	}
}
