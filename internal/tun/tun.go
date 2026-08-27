// Package tun provides cross-platform TUN device abstraction.
package tun

import (
	"errors"
	"fmt"
	"net"
)

// Device represents a TUN interface.
type Device interface {
	Name() string
	MTU() int
	Read(buf []byte) (int, error)
	Write(buf []byte) (int, error)
	IP() net.IP
	Close() error
}

// Config holds TUN creation parameters.
type Config struct {
	Name string
	MTU  int
	CIDR string // e.g. 10.88.0.1/24
}

// Open creates a platform-specific TUN device.
func Open(cfg Config) (Device, error) {
	if cfg.MTU <= 0 {
		cfg.MTU = 1420
	}
	if cfg.CIDR == "" {
		return nil, errors.New("tun CIDR required")
	}
	return openPlatform(cfg)
}

// ParseCIDR parses an IP network string.
func ParseCIDR(cidr string) (net.IP, *net.IPNet, error) {
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CIDR %q: %w", cidr, err)
	}
	return ip, n, nil
}
