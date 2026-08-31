//go:build windows

package winnet

import (
	"net"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestIPv4FromSockaddrInet 联合体 AF_INET 提取。
func TestIPv4FromSockaddrInet(t *testing.T) {
	var sa windows.RawSockaddrInet6
	type sockaddrIn struct {
		Family uint16
		Port   uint16
		Addr   [4]byte
		Zero   [8]byte
	}
	in := (*sockaddrIn)(unsafe.Pointer(&sa))
	in.Family = windows.AF_INET
	in.Addr = [4]byte{10, 88, 0, 87}
	ip := ipv4FromSockaddrInet(&sa)
	if ip == nil || !ip.Equal(net.IPv4(10, 88, 0, 87)) {
		t.Fatalf("got %v", ip)
	}
}

// TestPrefixLenToMask /32 /24。
func TestPrefixLenToMask(t *testing.T) {
	if got := prefixLenToMask(32); got != "255.255.255.255" {
		t.Fatalf("/32 got %s", got)
	}
	if got := prefixLenToMask(24); got != "255.255.255.0" {
		t.Fatalf("/24 got %s", got)
	}
}
