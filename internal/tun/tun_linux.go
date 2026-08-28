//go:build linux

package tun

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/platform"
)

type linuxDevice struct {
	name string
	mtu  int
	fd   *os.File
	ip   net.IP
}

func openPlatform(cfg Config) (Device, error) {
	fd, err := syscall.Open("/dev/net/tun", syscall.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/net/tun: %w", err)
	}
	name := cfg.Name
	if name == "" {
		name = brand.DefaultTunName
	}
	var req struct {
		Name  [16]byte
		Flags uint16
		_     [22]byte
	}
	copy(req.Name[:], name)
	req.Flags = syscall.IFF_TUN | syscall.IFF_NO_PI
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TUNSETIFF), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("TUNSETIFF: %v", errno)
	}
	actualName := string(req.Name[:])
	for i := 0; i < len(actualName); i++ {
		if actualName[i] == 0 {
			actualName = actualName[:i]
			break
		}
	}
	ip, ipNet, err := ParseCIDR(cfg.CIDR)
	if err != nil {
		syscall.Close(fd)
		return nil, err
	}
	ones, _ := ipNet.Mask.Size()
	// 必须真正配 IP 并 UP，否则路由/转发无效
	if out, err := exec.Command("ip", "addr", "replace", fmt.Sprintf("%s/%d", ip, ones), "dev", actualName).CombinedOutput(); err != nil {
		syscall.Close(fd)
		return nil, platform.CommandOutputError("ip addr", out, err)
	}
	if out, err := exec.Command("ip", "link", "set", "dev", actualName, "up", "mtu", fmt.Sprintf("%d", cfg.MTU)).CombinedOutput(); err != nil {
		syscall.Close(fd)
		return nil, platform.CommandOutputError("ip link up", out, err)
	}
	file := os.NewFile(uintptr(fd), "/dev/net/tun")
	dev := &linuxDevice{name: actualName, mtu: cfg.MTU, fd: file, ip: ip}
	logger.Info("linux tun %s created, ip=%s/%d mtu=%d", actualName, ip, ones, cfg.MTU)
	return dev, nil
}

func (d *linuxDevice) Name() string               { return d.name }
func (d *linuxDevice) MTU() int                   { return d.mtu }
func (d *linuxDevice) IP() net.IP                 { return d.ip }
func (d *linuxDevice) Read(b []byte) (int, error) { return d.fd.Read(b) }
func (d *linuxDevice) Write(b []byte) (int, error) {
	return d.fd.Write(b)
}
func (d *linuxDevice) Close() error {
	logger.Info("closing linux tun %s", d.name)
	return d.fd.Close()
}
