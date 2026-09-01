package netutil

import (
	"net"
	"testing"
)

func TestIPv4AddrsToRemove(t *testing.T) {
	got := IPv4AddrsToRemove([]string{"10.88.0.2", "10.88.0.4", "192.168.137.1"}, "10.88.0.4")
	if len(got) != 2 || got[0] != "10.88.0.2" || got[1] != "192.168.137.1" {
		t.Fatalf("%v", got)
	}
	if len(IPv4AddrsToRemove([]string{"10.88.0.4"}, "10.88.0.4")) != 0 {
		t.Fatal("same ip")
	}
}

func TestIPv4AddrsToRemoveKeepICS(t *testing.T) {
	got := IPv4AddrsToRemoveKeepICS([]string{"10.88.0.2", "10.88.0.4", "192.168.137.1"}, "10.88.0.4")
	if len(got) != 1 || got[0] != "10.88.0.2" {
		t.Fatalf("%v", got)
	}
}

func TestIPv4IsICSPrivate(t *testing.T) {
	if !IPv4IsICSPrivate(net.ParseIP("192.168.137.1")) {
		t.Fatal("137")
	}
	if IPv4IsICSPrivate(net.ParseIP("10.0.0.1")) {
		t.Fatal("not 137")
	}
	if ICSPrivateIPv4Wildcard() != "192.168.137.*" {
		t.Fatal("wildcard")
	}
}

func TestPreferSkipAsSourceNeedsUpdate(t *testing.T) {
	if PreferSkipAsSourceNeedsUpdate(false, true, true) {
		t.Fatal("already ok")
	}
	if !PreferSkipAsSourceNeedsUpdate(true, true, true) {
		t.Fatal("vpn skip")
	}
	if !PreferSkipAsSourceNeedsUpdate(false, true, false) {
		t.Fatal("137 skip false")
	}
}
