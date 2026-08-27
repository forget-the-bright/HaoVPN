// Package ippool manages VPN virtual IP allocation.
package ippool

import (
	"fmt"
	"net"
	"sync"
)

// Pool allocates IPs from a CIDR subnet.
type Pool struct {
	mu        sync.Mutex
	network   *net.IPNet
	reserved  map[string]bool
	allocated map[string]int64 // ip -> peerID
	next      net.IP
}

// New creates an IP pool from a CIDR string.
func New(cidr string) (*Pool, error) {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("parse subnet: %w", err)
	}
	start := make(net.IP, len(n.IP))
	copy(start, n.IP)
	// Skip network address
	incIP(start)
	return &Pool{
		network:   n,
		reserved:  map[string]bool{},
		allocated: map[string]int64{},
		next:      start,
	}, nil
}

// Reserve marks an IP as reserved (e.g. server gateway).
func (p *Pool) Reserve(ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reserved[ip] = true
}

// Allocate assigns the next free IP to peerID.
func (p *Pool) Allocate(peerID int64) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := 0; i < 256; i++ {
		ip := p.next.String()
		if !p.reserved[ip] && p.allocated[ip] == 0 && p.network.Contains(p.next) {
			p.allocated[ip] = peerID
			incIP(p.next)
			return ip, nil
		}
		incIP(p.next)
	}
	return "", fmt.Errorf("no free IPs in pool")
}

// AllocateSpecific assigns a specific IP if available（允许覆盖 peerID=0 的临时占用）。
func (p *Pool) AllocateSpecific(ip string, peerID int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	parsed := net.ParseIP(ip)
	if parsed == nil || !p.network.Contains(parsed) {
		return fmt.Errorf("IP %s not in pool", ip)
	}
	if p.reserved[ip] {
		return fmt.Errorf("IP %s already allocated", ip)
	}
	if owner, ok := p.allocated[ip]; ok && owner != 0 && owner != peerID {
		return fmt.Errorf("IP %s already allocated", ip)
	}
	p.allocated[ip] = peerID
	return nil
}

// Release returns an IP to the pool.
func (p *Pool) Release(ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.allocated, ip)
}

// IsAllocated checks if IP is in use（含 peerID=0 的临时占用）。
func (p *Pool) IsAllocated(ip string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.allocated[ip]
	return ok
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
}
