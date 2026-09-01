package clientapp

import "testing"

func TestMergeConnectWarns(t *testing.T) {
	if got := MergeConnectWarns("", "ics"); got != "ics" {
		t.Fatalf("got %q", got)
	}
	if got := MergeConnectWarns("route", ""); got != "route" {
		t.Fatalf("got %q", got)
	}
	if got := MergeConnectWarns("route", "ics"); got != "route\nics" {
		t.Fatalf("got %q", got)
	}
}
