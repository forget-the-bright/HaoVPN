//go:build darwin

package tun

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"unsafe"

	"haovpn/internal/logger"
	"haovpn/internal/platform"
)

const (
	syscallConnect = 0x2a
)

type darwinDevice struct {
	name string
	mtu  int
	ip   net.IP
	fd   *os.File
}

func openPlatform(cfg Config) (Device, error) {
	fd, err := syscall.Socket(syscall.AF_SYSTEM, syscall.SOCK_DGRAM, syscallConnect)
	if err != nil {
		return nil, fmt.Errorf("socket: %w", err)
	}
	for i := 0; i < 255; i++ {
		name := fmt.Sprintf("utun%d", i)
		copyName := [16]byte{}
		copy(copyName[:], name)
		_, _, errno := syscall.Syscall(syscall.SYS_CONNECT, uintptr(fd), uintptr(unsafe.Pointer(&copyName[0])), uintptr(len(name)))
		if errno != 0 {
			continue
		}
		file := os.NewFile(uintptr(fd), name)
		ip, ipNet, err := parseCIDR(cfg.CIDR)
		if err != nil {
			file.Close()
			return nil, err
		}
		ones, _ := ipNet.Mask.Size()
		// utun 必须 ifconfig 配地址，否则无 IP
		cmd := platform.Command("ifconfig", name, "inet", ip.String(), ip.String(), "netmask", net.IP(ipNet.Mask).String(), "mtu", fmt.Sprintf("%d", cfg.MTU), "up")
		if out, err := cmd.CombinedOutput(); err != nil {
			file.Close()
			return nil, platform.CommandOutputError("ifconfig "+name, out, err)
		}
		logger.Info("darwin utun %s created, ip=%s/%d mtu=%d", name, ip, ones, cfg.MTU)
		return &darwinDevice{name: name, mtu: cfg.MTU, ip: ip, fd: file}, nil
	}
	syscall.Close(fd)
	return nil, fmt.Errorf("no available utun device")
}

func (d *darwinDevice) Name() string               { return d.name }
func (d *darwinDevice) MTU() int                   { return d.mtu }
func (d *darwinDevice) IP() net.IP                 { return d.ip }
func (d *darwinDevice) Read(b []byte) (int, error) { return d.fd.Read(b) }
func (d *darwinDevice) Write(b []byte) (int, error) {
	return d.fd.Write(b)
}
func (d *darwinDevice) Close() error {
	logger.Info("closing darwin utun %s", d.name)
	return d.fd.Close()
}
