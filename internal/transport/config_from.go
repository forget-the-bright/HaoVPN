package transport

import (
	"time"

	"haovpn/internal/config"
	"haovpn/internal/netutil"
)

// FromClientConfig 从客户端 YAML 构造 transport.Config（心跳/重连/拨号/MTU 单一映射源）。
func FromClientConfig(c *config.ClientConfig) Config {
	if c == nil {
		return DefaultConfig()
	}
	c.ApplyDefaults()
	tcfg := DefaultConfig()
	tcfg.ReconnectInitial = time.Duration(c.Reconnect.InitialSec) * time.Second
	tcfg.ReconnectMax = time.Duration(c.Reconnect.MaxSec) * time.Second
	tcfg.DialTimeout = time.Duration(c.Server.DialTimeoutSec) * time.Second
	tcfg.HeartbeatInterval = time.Duration(c.Server.HeartbeatIntervalSec) * time.Second
	tcfg.HeartbeatTimeout = time.Duration(c.Server.HeartbeatTimeoutSec) * time.Second
	tcfg.MTU = netutil.ResolveMTU(c.Tun.MTU)
	return tcfg
}

// FromServerVPN 从服务端 vpn 段构造 transport.Config（调用方须已 ApplyDefaults）。
func FromServerVPN(vpn config.VPNSection) Config {
	tcfg := DefaultConfig()
	tcfg.HeartbeatInterval = time.Duration(vpn.HeartbeatIntervalSec) * time.Second
	tcfg.HeartbeatTimeout = time.Duration(vpn.HeartbeatTimeoutSec) * time.Second
	tcfg.MTU = netutil.ResolveMTU(vpn.MTU)
	return tcfg
}
