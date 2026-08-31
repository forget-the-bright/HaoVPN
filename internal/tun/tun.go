package tun

import (
	"errors"
	"net"

	"haovpn/internal/netutil"
)

// Device 抽象平台 TUN 虚拟网卡：读写 IP 包、查询 MTU 与地址。
//
// 实现方：Windows 为 Wintun；其他平台见 tun_*.go。
// 生命周期：Open 成功后须 Close；Close 后 Read/Write 行为未定义。
// 并发：具体实现是否允许多 goroutine 读写见平台文档；调用方通常单读单写。
type Device interface {
	Name() string
	MTU() int
	Read(buf []byte) (int, error)
	Write(buf []byte) (int, error)
	IP() net.IP
	Close() error
}

// Config 创建 TUN 设备时的名称、MTU 与接口地址。
//
// 字段：
//   Name — 期望接口名；空则由平台分配（如 Wintun 池名）。
//   MTU — 接口 MTU；≤0 时 Open 调用 netutil.ResolveMTU()。
//   CIDR — 本端 TUN 地址，如 10.88.0.1/24；必填。
type Config struct {
	Name string
	MTU  int
	CIDR string // e.g. 10.88.0.1/24
}

// Open 按平台创建 TUN 设备并完成基础参数校验。
//
// 参数：cfg.CIDR 非空；cfg.MTU ≤0 时自动解析默认 MTU。
// 返回：Device 实现与 err；CIDR 缺失或平台 open 失败时 Device 为 nil。
// 副作用：可能创建系统虚拟网卡；通常需管理员权限。
func Open(cfg Config) (Device, error) {
	if cfg.MTU <= 0 {
		cfg.MTU = netutil.ResolveMTU()
	}
	if cfg.CIDR == "" {
		return nil, errors.New("tun CIDR required")
	}
	return openPlatform(cfg)
}

// WarmupAdapter 在登录前确保名为 name 的 Wintun 适配器已存在（Open 或 Create）。
//
// 参数：name — yaml tun.name；空则用品牌默认名。
// 返回：平台创建失败时 error；非 Windows 为 noop nil。
// 副作用：可能 CreateAdapter（公司机冷创建可数十秒）；关闭句柄但保留适配器，供随后 Open 走「已复用」。
// 调用方：clientgui 托盘启动后台；失败仅 Warn，不阻断登录。
func WarmupAdapter(name string) error {
	return warmupPlatform(name)
}

// parseCIDR 解析 CIDR 并返回主机 IP 与网段（委托 netutil）。
//
// 故意不导出：外部请直接调 netutil.ParseCIDR，避免再造一层薄 re-export。
func parseCIDR(cidr string) (net.IP, *net.IPNet, error) {
	return netutil.ParseCIDR(cidr)
}
