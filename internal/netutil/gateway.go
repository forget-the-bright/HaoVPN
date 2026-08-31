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
//
// 与 InferVPNSubnetHint 共用「默认 /24、主机落在 x.y.z.0/24」假设；改子网启发式须两边一起审。
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

// InferVPNSubnetHint 从客户端 VPN IPv4 粗推同网段 x.y.z.0/24（展示/去重回退用）。
//
// 用途：握手未带 vpn_subnet 时，GUI 托盘「本机 TUN」行与分流去重键需要一个子网提示。
// 不是路由权威来源——真实子网以握手 vpn_subnet / 服务端池为准。
//
// 参数：vpnIP — 如 "10.88.0.5"；有效 IPv4 返回 "10.88.0.0/24"；
// 无效时返回 TrimSpace(vpnIP)（保留原展示回退，避免托盘空白）。
//
// 关联：InferGatewayFromVPNIP（同 /24 假设）；调用方 clientgui/tray_routes.go。
func InferVPNSubnetHint(vpnIP string) string {
	raw := strings.TrimSpace(vpnIP)
	ip := net.ParseIP(raw)
	if ip == nil {
		return raw
	}
	v4 := ip.To4()
	if v4 == nil {
		return raw
	}
	return fmt.Sprintf("%d.%d.%d.0/24", v4[0], v4[1], v4[2])
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
