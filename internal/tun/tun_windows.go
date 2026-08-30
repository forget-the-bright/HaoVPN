//go:build windows

package tun

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"

	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/tun/wintundll"
	"haovpn/internal/winnet"
)

// winDevice Windows Wintun 适配器封装，实现 Device 接口。
//
// 字段：
//   mu — 保护 session 读写与 closed 标志。
//   name — 适配器配置名（如 haovpn0）。
//   mtu/ip/cidr — Open 时写入的配置快照。
//   ifIndex — 网卡索引；路由 netsh 命令使用。
//   reused — true 表示 OpenAdapter 复用已有适配器（快速重连）。
//   adapter/session — Wintun 原生句柄。
//   readWait — ReceivePacket 无数据时的等待事件。
type winDevice struct {
	mu       sync.Mutex
	name     string
	mtu      int
	ip       net.IP
	cidr     string
	ifIndex  int
	reused   bool
	adapter  *wintun.Adapter
	session  wintun.Session
	readWait windows.Handle
	closed   bool
}

// openPlatform Windows 平台 Open 实现：Ensure DLL → Open/Create 适配器 → 配 IP。
func openPlatform(cfg Config) (Device, error) {
	if err := wintundll.Ensure(); err != nil {
		return nil, err
	}
	name := cfg.Name
	if name == "" {
		name = brand.DefaultTunName
	}
	ip, ipNet, err := parseCIDR(cfg.CIDR)
	if err != nil {
		return nil, err
	}
	adapter, reused, err := openWintunAdapter(name)
	if err != nil {
		return nil, err
	}
	session, err := adapter.StartSession(0x800000)
	if err != nil {
		adapter.Close()
		return nil, fmt.Errorf("wintun session: %w", err)
	}

	d := &winDevice{
		name:     name,
		mtu:      cfg.MTU,
		ip:       ip,
		cidr:     cfg.CIDR,
		reused:   reused,
		adapter:  adapter,
		session:  session,
		readWait: session.ReadWaitEvent(),
	}
	winnet.RegisterFromLUID(name, adapter.LUID())
	if idx, err := winnet.InterfaceIndex(name); err == nil {
		d.ifIndex = idx
	} else {
		logger.Debug("Wintun 登记后查 ifIndex 失败: %v", err)
	}
	if err := d.assignIPv4(ip, ipNet); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("assign TUN IP: %w", err)
	}
	if reused {
		logger.Info("windows wintun %s 已复用, ip=%s mtu=%d", name, ip, cfg.MTU)
	} else {
		logger.Info("windows wintun %s created, ip=%s mtu=%d", name, ip, cfg.MTU)
	}
	return d, nil
}

// assignIPv4 为 TUN 配置 IPv4；已存在则跳过；netsh 失败时回退 PowerShell。
func (d *winDevice) assignIPv4(ip net.IP, ipNet *net.IPNet) error {
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return fmt.Errorf("仅支持 IPv4: bits=%d", bits)
	}
	mask := net.IP(ipNet.Mask).String()
	ifName := winnet.ResolveInterfaceAlias(d.name)
	if winnet.InterfaceHasIPv4(d.name, d.ifIndex, ip.String()) {
		logger.Info("windows TUN IP 已就绪，跳过重复配置: %s ifIndex=%d", ip, d.ifIndex)
		if !d.reused {
			d.disableIPv6BestEffort(ifName)
		}
		return nil
	}
	if err := winnet.SetInterfaceIPv4(ifName, ip.String(), mask); err != nil {
		logger.Warn("netsh 配置 TUN IP 失败，尝试 PowerShell: %v", err)
		if err2 := winnet.AssignIPv4PowerShell(d.name, ip.String(), ones); err2 != nil {
			if !winnet.InterfaceHasIPv4(d.name, d.ifIndex, ip.String()) {
				return fmt.Errorf("netsh: %v; powershell: %w", err, err2)
			}
			logger.Warn("TUN IP 配置命令失败但系统已存在 %s，继续启动: netsh=%v ps=%v", ip, err, err2)
		}
	}
	if err := d.waitTunIPv4Ready(ip, 1500*time.Millisecond); err != nil {
		return fmt.Errorf("TUN IP 未就绪: %w", err)
	}
	if !d.reused {
		d.disableIPv6BestEffort(ifName)
	}
	logger.Info("windows TUN IP 已配置: %s/%d on %s", ip, ones, d.name)
	return nil
}

// waitTunIPv4Ready 轮询直到系统可见 TUN IPv4 或超时。
func (d *winDevice) waitTunIPv4Ready(ip net.IP, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if winnet.InterfaceHasIPv4(d.name, d.ifIndex, ip.String()) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("超时 %s 内未看到 %s (ifIndex=%d)", timeout, ip, d.ifIndex)
}

// disableIPv6BestEffort 尽力禁用 TUN IPv6，避免分流异常；失败仅 Debug。
func (d *winDevice) disableIPv6BestEffort(ifName string) {
	if err := winnet.DisableInterfaceIPv6(ifName); err != nil {
		logger.Debug("禁用 TUN IPv6 未成功（可忽略）: %v", err)
		return
	}
	logger.Info("已禁用 TUN IPv6: %s", d.name)
}

func (d *winDevice) Name() string { return d.name }
func (d *winDevice) MTU() int     { return d.mtu }
func (d *winDevice) IP() net.IP   { return d.ip }

// Read 从 Wintun 会话读 IP 包；无数据时等待 readWait 事件。
func (d *winDevice) Read(b []byte) (int, error) {
	for {
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return 0, os.ErrClosed
		}
		pkt, err := d.session.ReceivePacket()
		switch err {
		case nil:
			n := copy(b, pkt)
			d.session.ReleaseReceivePacket(pkt)
			d.mu.Unlock()
			return n, nil
		case windows.ERROR_NO_MORE_ITEMS:
			d.mu.Unlock()
			for {
				if d.isClosed() {
					return 0, os.ErrClosed
				}
				ev, e := windows.WaitForSingleObject(d.readWait, 200)
				if e != nil {
					return 0, e
				}
				if ev == windows.WAIT_OBJECT_0 {
					break
				}
			}
			continue
		case windows.ERROR_HANDLE_EOF:
			d.mu.Unlock()
			return 0, os.ErrClosed
		default:
			d.mu.Unlock()
			return 0, fmt.Errorf("wintun receive: %w", err)
		}
	}
}

func (d *winDevice) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

// Write 向 Wintun 写入 IP 包。
func (d *winDevice) Write(b []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return 0, os.ErrClosed
	}
	pkt, err := d.session.AllocateSendPacket(len(b))
	if err != nil {
		return 0, err
	}
	copy(pkt, b)
	d.session.SendPacket(pkt)
	return len(b), nil
}

// Close 结束 Wintun 会话但保留适配器供下次快速 Open（设计选择）。
func (d *winDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	logger.Info("closing windows wintun session %s (保留适配器供下次快速复用)", d.name)
	if d.readWait != 0 {
		_ = windows.SetEvent(d.readWait)
	}
	d.session.End()
	runtime.SetFinalizer(d.adapter, nil)
	d.adapter = nil
	return nil
}
