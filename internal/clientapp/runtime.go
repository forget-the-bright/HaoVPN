package clientapp

import (
	"net"
	"sync"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logger"
	"haovpn/internal/tun"
)

// runtime.go：客户端 TUN/路由运行时结构体与生命周期（close/write/allowedIPs）。
// 策略应用、路由差分、TUN 上送分别见 runtime_policy / runtime_routes / runtime_tun。

// runtime 客户端 TUN 设备与系统路由/DNS 的运行时状态（Engine 内聚，不对外导出）。
//
// 字段：
//   mu — 保护 tunDev、routes、allowedCIDRs、vpnIP、policyVer、gateway、via、appliedDNS。
//   cfg — 客户端配置引用，用于 TUN 名、MTU、DNS 开关等。
//   tunDev — 已打开的 TUN 设备；vpnIP 变化时可能关闭并重建。
//   routes — 已通过 netstack 添加的路由 CIDR 列表（规范化），断线临时重连时保留。
//   allowedCIDRs — 最近一次握手策略中的 AllowedIPs，供杀开关前缀使用。
//   vpnIP — 当前 TUN 绑定的虚拟 IP。
//   policyVer — 服务端策略版本号，变更时打日志。
//   gateway — 当前用于 AddClientRoute 的下一跳网关 IP。
//   via — local_lans 非空时的 via 出口 Stack；空配置时为 nil。
//   viaFP — 当前已 Setup 的 via 指纹；与握手配置相同则跳过 ICS 重建。
//   appliedDNS — 最近一次成功写入的 DNS 列表（客户端侧缓存）。
//   exitLANNets — 解析后的 local_lans，供 TUN 上送过滤（允许 LAN 回程源）。
type runtime struct {
	mu           sync.Mutex
	cfg          *config.ClientConfig
	tunDev       tun.Device
	routes       []string
	allowedCIDRs []string
	vpnIP        string
	policyVer    int
	gateway      string
	via          *viaExit
	viaFP        string
	viaFPKnown   bool // 是否已成功应用过 via 状态（区分「从未应用」与「via 关闭」）
	appliedDNS   []string
	exitLANNets  []*net.IPNet
}

// allowedIPs 返回 AllowedIPs 副本，供杀开关 Enable 使用。
func (rt *runtime) allowedIPs() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string{}, rt.allowedCIDRs...)
}

func (rt *runtime) close() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	start := time.Now()
	logger.Info("dataplane_clear reason=stop")
	rt.clearRoutesLocked()
	if rt.tunDev != nil {
		_ = rt.tunDev.Close()
		rt.tunDev = nil
	}
	rt.allowedCIDRs = nil
	logger.Info("dataplane_clear done elapsed=%s", time.Since(start))
}

func (rt *runtime) write(pkt []byte) error {
	rt.mu.Lock()
	dev := rt.tunDev
	rt.mu.Unlock()
	if dev == nil {
		return nil
	}
	_, err := dev.Write(pkt)
	return err
}
