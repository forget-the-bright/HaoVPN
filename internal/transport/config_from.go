package transport

import (
	"haovpn/internal/config"
	"haovpn/internal/netutil"
	"haovpn/internal/timeutil"
)

// FromClientConfig 从客户端 YAML 构造 transport.Config（心跳/重连/拨号/MTU/队列单一映射源）。
func FromClientConfig(c *config.ClientConfig) Config {
	if c == nil {
		return DefaultConfig()
	}
	c.ApplyDefaults()
	tcfg := DefaultConfig()
	tcfg.ReconnectInitial = timeutil.Seconds(c.Reconnect.InitialSec)
	tcfg.ReconnectMax = timeutil.Seconds(c.Reconnect.MaxSec)
	tcfg.DialTimeout = timeutil.Seconds(c.Server.DialTimeoutSec)
	tcfg.HeartbeatInterval = timeutil.Seconds(c.Server.HeartbeatIntervalSec)
	tcfg.HeartbeatTimeout = timeutil.Seconds(c.Server.HeartbeatTimeoutSec)
	tcfg.MTU = netutil.ResolveMTU(c.Tun.MTU)
	tcfg.MaxQueueSize = c.Server.SendQueueSize
	return tcfg
}

// FromServerVPN 从服务端 vpn 段构造 transport.Config（调用方须已 ApplyDefaults）。
func FromServerVPN(vpn config.VPNSection) Config {
	tcfg := DefaultConfig()
	tcfg.HeartbeatInterval = timeutil.Seconds(vpn.HeartbeatIntervalSec)
	tcfg.HeartbeatTimeout = timeutil.Seconds(vpn.HeartbeatTimeoutSec)
	tcfg.MTU = netutil.ResolveMTU(vpn.MTU)
	tcfg.MaxQueueSize = vpn.SendQueueSize
	return tcfg
}
