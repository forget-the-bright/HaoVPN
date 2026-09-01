package serverapp

import (
	"context"

	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/netutil"
	"haovpn/internal/tun"
)

// boot_tun.go：服务端 TUN 打开与 netstack 路由/NAT Setup。

// bootTUNNetstack 创建 TUN 并配置路由/NAT。
func bootTUNNetstack(bc *bootContext) (*tunNetstackResult, error) {
	cfg := bc.cfg
	res := &tunNetstackResult{natOK: !cfg.NAT.Enabled}
	gatewayCIDR := netutil.GatewayCIDR(cfg.VPN.GatewayIP, cfg.VPN.Subnet)
	tunDev, err := tun.Open(tun.Config{Name: brand.DefaultTunName, MTU: cfg.VPN.MTU, CIDR: gatewayCIDR})
	if err != nil {
		if cfg.VPN.RequireTun {
			return nil, err
		}
		logger.Warn("TUN 创建失败（需管理员权限：sudo 或提权终端）: %v", err)
		return res, nil
	}
	res.tunDev = tunDev
	res.tunOK = true

	ns := netstack.New(netstack.Config{
		TunName:     tunDev.Name(),
		TunIP:       tunDev.IP(),
		VPNSubnet:   cfg.VPN.Subnet,
		LanCIDRs:    cfg.NAT.AllowedLANCIDRs,
		OutboundIf:  cfg.NAT.OutboundInterface,
		ForwardOnly: cfg.NAT.ForwardOnly,
		Enabled:     cfg.NAT.Enabled,
	})
	if err := ns.Setup(context.Background()); err != nil {
		if cfg.NAT.Enabled {
			_ = tunDev.Close()
			return nil, err
		}
		logger.Error("netstack Setup 失败: %v", err)
	} else if ns.SNATEnabled() {
		res.natOK = true
	}
	res.ns = ns
	return res, nil
}
