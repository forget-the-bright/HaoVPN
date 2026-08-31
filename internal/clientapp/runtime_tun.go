package clientapp

import (
	"context"
	"net"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
)

// runtime_tun.go：TUN 读循环、上送过滤与 local_lans 网段缓存。
//
// 读循环内 time.Sleep：设备未就绪或读错误时的持续轮询（有 ctx 生命周期），
// 非「有限次 settle」——勿改成 safeutil.RetryN（会改变语义为失败退出）。

// cacheExitLANNetsLocked 解析 local_lans 供 TUN 上送过滤；调用方须已持 rt.mu。
func (rt *runtime) cacheExitLANNetsLocked() {
	rt.exitLANNets = nil
	if rt.cfg == nil {
		return
	}
	// 与握手/出口一致：先 ValidLANCIDRs，再解析；空则关闭上送放宽
	lans := netutil.ValidLANCIDRs(rt.cfg.LocalLANs)
	if len(lans) == 0 {
		return
	}
	nets, err := netutil.ParseCIDRListToNets(lans)
	if err != nil {
		return
	}
	rt.exitLANNets = nets
}

func (rt *runtime) readLoop(ctx context.Context, send func([]byte) error, mtu int) {
	buf := make([]byte, netutil.ReadBufferSize(mtu))
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		rt.mu.Lock()
		dev := rt.tunDev
		rt.mu.Unlock()
		if dev == nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		n, err := dev.Read(buf)
		if err != nil {
			logger.Warn("TUN 读错误: %v", err)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		pkt := buf[:n]
		if !rt.shouldUploadTUN(pkt) {
			continue
		}
		if err := send(pkt); err != nil {
			logger.Warn("隧道发送失败: %v", err)
		}
	}
}

// shouldUploadTUN 仅上送合法 IPv4：源为本机 VPN IP，或落在 local_lans（via 回程）。
// 过滤 ICS(192.168.137.x)、广播/组播等误注入，避免服务端伪造源刷屏。
func (rt *runtime) shouldUploadTUN(pkt []byte) bool {
	if len(pkt) < 20 || pkt[0]>>4 != 4 {
		return false
	}
	src := net.IP(pkt[12:16])
	dst := net.IP(pkt[16:20])
	if dst != nil && (dst.IsMulticast() || dst.IsLinkLocalMulticast() || netutil.IsLimitedBroadcast(dst)) {
		return false
	}
	rt.mu.Lock()
	vpnIP := rt.vpnIP
	lans := rt.exitLANNets
	rt.mu.Unlock()
	if vpnIP != "" && src.String() == vpnIP {
		return true
	}
	for _, n := range lans {
		if n != nil && n.Contains(src) {
			return true
		}
	}
	return false
}
