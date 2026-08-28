package netutil

import (
	"fmt"
	"net"
	"strings"
)

// defaultGatewayFallback 无法从 vpnIP 解析网关时使用的默认值（与历史 client.yaml 子网一致）。
const defaultGatewayFallback = "10.88.0.1"

// InferGatewayFromVPNIP 从客户端分配的 VPN IPv4 推断同网段 .1 网关。
//
// 参数：vpnIP — 如 "10.88.0.5"；无效或非 IPv4 时回退 defaultGatewayFallback。
// 返回：如 "10.88.0.1"。
func InferGatewayFromVPNIP(vpnIP string) string {
	ip := net.ParseIP(strings.TrimSpace(vpnIP))
	if ip == nil {
		return defaultGatewayFallback
	}
	v4 := ip.To4()
	if v4 == nil {
		return defaultGatewayFallback
	}
	return fmt.Sprintf("%d.%d.%d.1", v4[0], v4[1], v4[2])
}

// ResolveGateway 解析客户端路由下一跳网关地址。
//
// 优先级：握手 gateway_ip > InferGatewayFromVPNIP(vpnIP)。yamlGW 参数保留供兼容调用，客户端 yaml 不再使用。
func ResolveGateway(handshakeGW, yamlGW, vpnIP string) string {
	if g := strings.TrimSpace(handshakeGW); g != "" {
		return g
	}
	if g := strings.TrimSpace(yamlGW); g != "" {
		return g
	}
	return InferGatewayFromVPNIP(vpnIP)
}

// IsLoopbackHost 判断 host 是否为环回或 localhost 字面量。
//
// 用途：管理 API 绑定校验，防止误将非环回地址当作本地管理口。
func IsLoopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
