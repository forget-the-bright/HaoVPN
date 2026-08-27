//go:build windows

package tun

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"

	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/platform"
)

type winDevice struct {
	mu       sync.Mutex
	name     string
	mtu      int
	ip       net.IP
	cidr     string
	adapter  *wintun.Adapter
	session  wintun.Session
	readWait windows.Handle
	closed   bool
}

func openPlatform(cfg Config) (Device, error) {
	name := cfg.Name
	if name == "" {
		name = brand.DefaultTunName
	}
	ip, ipNet, err := ParseCIDR(cfg.CIDR)
	if err != nil {
		return nil, err
	}
	adapter, err := wintun.CreateAdapter(name, brand.WintunPool, nil)
	if err != nil {
		return nil, fmt.Errorf("wintun create: %w", err)
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
		adapter:  adapter,
		session:  session,
		readWait: session.ReadWaitEvent(),
	}
	// Wintun 只建适配器，不会自动配 IP；须写入系统路由栈，否则 bind TUN IP 会失败
	if err := d.assignIPv4(ip, ipNet); err != nil {
		_ = d.Close()
		return nil, fmt.Errorf("assign TUN IP: %w", err)
	}
	logger.Info("windows wintun %s created, ip=%s mtu=%d", name, ip, cfg.MTU)
	return d, nil
}

// assignIPv4 给 Wintun 网卡配置静态 IPv4（需管理员）。netsh 失败时回退 PowerShell。
func (d *winDevice) assignIPv4(ip net.IP, ipNet *net.IPNet) error {
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return fmt.Errorf("仅支持 IPv4: bits=%d", bits)
	}
	mask := net.IP(ipNet.Mask).String()
	// 重启后 IP 可能仍挂在残留 Wintun 网卡上；若路由栈已可见则不必重复 netsh（避免 5010/对象已存在）
	if err := d.waitIPv4Ready(ip.String(), 2*time.Second); err == nil {
		logger.Info("windows TUN IP 已就绪，跳过重复配置: %s on %s", ip, d.name)
		d.disableIPv6BestEffort()
		return nil
	}
	if err := d.assignIPv4Netsh(ip.String(), mask); err != nil {
		logger.Warn("netsh 配置 TUN IP 失败，尝试 PowerShell: %v", err)
		if err2 := d.assignIPv4PowerShell(ip.String(), ones); err2 != nil {
			if waitErr := d.waitIPv4Ready(ip.String(), 3*time.Second); waitErr == nil {
				logger.Warn("TUN IP 配置命令失败但系统已存在 %s，继续启动: netsh=%v ps=%v", ip, err, err2)
			} else {
				return fmt.Errorf("netsh: %v; powershell: %w", err, err2)
			}
		}
	}
	if err := d.waitIPv4Ready(ip.String(), 5*time.Second); err != nil {
		return fmt.Errorf("TUN IP 未就绪: %w", err)
	}
	d.disableIPv6BestEffort()
	logger.Info("windows TUN IP 已配置: %s/%d on %s", ip, ones, d.name)
	return nil
}

// disableIPv6BestEffort 关闭 TUN 上的 IPv6，避免 Windows 探测包经隧道刷「非 IPv4」日志。
func (d *winDevice) disableIPv6BestEffort() {
	cmd := platform.Command("netsh", "interface", "ipv6", "set", "interface",
		"interface="+d.name, "admin=disabled")
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Debug("禁用 TUN IPv6 未成功（可忽略）: %v %s", err, strings.TrimSpace(string(out)))
		return
	}
	logger.Info("已禁用 TUN IPv6: %s", d.name)
}

func (d *winDevice) assignIPv4Netsh(ip, mask string) error {
	cmd := platform.Command("netsh", "interface", "ipv4", "set", "address",
		"name="+d.name,
		"source=static",
		"addr="+ip,
		"mask="+mask,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh set address: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// assignIPv4PowerShell 按网卡名或 Wintun 描述查找适配器并配 IP（兼容中文系统网卡名不一致）。
func (d *winDevice) assignIPv4PowerShell(ip string, prefix int) error {
	ps := fmt.Sprintf(`
$if = Get-NetAdapter | Where-Object { $_.Name -eq '%s' } | Select-Object -First 1
if (-not $if) {
  $if = Get-NetAdapter | Where-Object { $_.InterfaceDescription -match 'Wintun|HaoVPN' } | Select-Object -First 1
}
if (-not $if) { throw '未找到 Wintun 网卡' }
Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Remove-NetIPAddress -Confirm:$false -ErrorAction SilentlyContinue
New-NetIPAddress -InterfaceIndex $if.ifIndex -IPAddress '%s' -PrefixLength %d -ErrorAction Stop | Out-Null
`, d.name, ip, prefix)
	out, err := platform.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// waitIPv4Ready 轮询直到系统路由栈可见 TUN IP，避免 bind 抢跑（不用 PowerShell，减少 GUI 连接卡顿）。
func (d *winDevice) waitIPv4Ready(ip string, timeout time.Duration) error {
	want := net.ParseIP(ip)
	if want == nil {
		return fmt.Errorf("无效 IP: %s", ip)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tunInterfaceHasIPv4(d.name, want) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("超时 %s 内未看到 %s", timeout, ip)
}

// tunInterfaceHasIPv4 检查指定网卡是否已绑定目标 IPv4。
func tunInterfaceHasIPv4(ifName string, ip net.IP) bool {
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		switch v := a.(type) {
		case *net.IPNet:
			if v.IP.Equal(ip) {
				return true
			}
		case *net.IPAddr:
			if v.IP.Equal(ip) {
				return true
			}
		}
	}
	return false
}

func (d *winDevice) Name() string { return d.name }
func (d *winDevice) MTU() int     { return d.mtu }
func (d *winDevice) IP() net.IP   { return d.ip }

// Read 从 Wintun 读 IP 包。空队列时 WaitForSingleObject；Close 与 ReceivePacket 须互斥，避免 End 时 ACCESS_VIOLATION。
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

func (d *winDevice) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	logger.Info("closing windows wintun %s", d.name)
	if d.readWait != 0 {
		_ = windows.SetEvent(d.readWait)
	}
	d.session.End()
	d.adapter.Close()
	return nil
}
