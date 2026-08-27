package ippool_test

import (
	"testing"

	"haovpn/internal/ippool"
)

func TestAllocateAndRelease(t *testing.T) {
	p, err := ippool.New("10.88.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	p.Reserve("10.88.0.1")
	ip, err := p.Allocate(1)
	if err != nil {
		t.Fatal(err)
	}
	if ip == "10.88.0.1" {
		t.Fatal("should skip reserved")
	}
	p.Release(ip)
	if p.IsAllocated(ip) {
		t.Fatal("should be released")
	}
}
