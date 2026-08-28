package serverapp

import (
	"net/http"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
	"haovpn/internal/transport"
	"haovpn/internal/tun"
)

// shutdownServer 优雅关闭管理 API、隧道与 TUN，并等待子 goroutine 结束。
func shutdownServer(sd *safeutil.Shutdown, apiServers []*http.Server, tunnelSrv *transport.Server, tunDev tun.Device, waitSec time.Duration) {
	logger.Info("正在优雅关闭...")
	for _, srv := range apiServers {
		_ = srv.Close()
	}
	if tunnelSrv != nil {
		_ = tunnelSrv.Close()
	}
	if tunDev != nil {
		_ = tunDev.Close()
	}
	sd.Wait(waitSec)
	logger.Info("优雅关闭完成")
}

func tunIPString(dev tun.Device) string {
	if dev == nil {
		return ""
	}
	return dev.IP().String()
}
