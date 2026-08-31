package clientapp

import "haovpn/internal/tunnel"

// ManagedRouteView 托盘/CLI 展示的托管路由一行（与 tunnel.ManagedRoute 字段对齐，避免 GUI 依赖 tunnel）。
type ManagedRouteView struct {
	Dest        string
	ViaIP       string
	ViaUserID   int64
	ViaUsername string
	Stale       bool
}

// ManagedRouteFromTunnel 将握手托管路由转为展示 DTO。
func ManagedRouteFromTunnel(m tunnel.ManagedRoute) ManagedRouteView {
	return ManagedRouteView{
		Dest: m.Dest, ViaIP: m.ViaIP, ViaUserID: m.ViaUserID,
		ViaUsername: m.ViaUsername, Stale: m.Stale,
	}
}

// ManagedRoutesFromTunnel 批量转换托管路由为展示 DTO。
func ManagedRoutesFromTunnel(in []tunnel.ManagedRoute) []ManagedRouteView {
	if len(in) == 0 {
		return nil
	}
	out := make([]ManagedRouteView, len(in))
	for i, m := range in {
		out[i] = ManagedRouteFromTunnel(m)
	}
	return out
}
