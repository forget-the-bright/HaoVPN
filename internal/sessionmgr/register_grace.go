package sessionmgr

import (
	"time"

	"haovpn/internal/netutil"
)

// register_grace.go：同主机 / 半死会话顶替判定（RegisterVPN reject_second 路径）。
// 无 I/O；仅依赖 PacketConn 可选接口与 netutil 主机规范化。

// sameRemoteHost 比较两个 host:port 的主机部分是否相同（忽略端口；规范化 IPv4/IPv6/本机回环）。
func sameRemoteHost(a, b string) bool {
	ha := netutil.NormalizeRemoteHost(a)
	hb := netutil.NormalizeRemoteHost(b)
	return ha != "" && ha == hb
}

// reconnectStaleAfter 半死会话判定阈值：对端静默超过此时间则允许密码重连顶替。
// 取 grace 与 20s 中较小者（至少 8s），以便客户端持续重试窗口内能顶替 ZT 黑洞会话。
//
// 刻意留在本包：仅 sessionmgr 注册路径使用，与传输层 HeartbeatTimeout / SCM 停服超时语义不同。
func reconnectStaleAfter(grace time.Duration) time.Duration {
	const minStale = 8 * time.Second
	const maxStale = 20 * time.Second
	if grace <= 0 {
		return maxStale
	}
	d := grace
	if d > maxStale {
		d = maxStale
	}
	if d < minStale {
		d = minStale
	}
	return d
}

// peerActivityStale 旧连接对端是否已静默超过阈值（实现 PeerActivityConn 时才可判定）。
func peerActivityStale(conn PacketConn, after time.Duration) bool {
	if conn == nil || after <= 0 {
		return false
	}
	pa, ok := conn.(PeerActivityConn)
	if !ok {
		return false
	}
	t := pa.LastPeerActivity()
	if t.IsZero() {
		return false
	}
	return time.Since(t) >= after
}
