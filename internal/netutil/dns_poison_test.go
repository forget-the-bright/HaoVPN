package netutil

import "testing"

func TestDNSServersPoisoned(t *testing.T) {
	if !DNSServersPoisoned([]string{"10.88.0.2", "8.8.8.8"}, []string{"10.88.0.2"}) {
		t.Fatal("poisoned")
	}
	if DNSServersPoisoned([]string{"8.8.8.8"}, []string{"10.88.0.2"}) {
		t.Fatal("clean")
	}
}

func TestFilterDNSServersPoison(t *testing.T) {
	got := FilterDNSServersPoison([]string{"10.88.0.2", "1.1.1.1"}, []string{"10.88.0.2"})
	if len(got) != 1 || got[0] != "1.1.1.1" {
		t.Fatalf("%v", got)
	}
}
