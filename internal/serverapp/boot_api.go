package serverapp

import (
	"context"
	"net/http"

	"haovpn/internal/api"
	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/maintenance"
	"haovpn/internal/netutil"
	"haovpn/internal/safeutil"
	"haovpn/internal/tun"
)

// boot_api.go：管理 API 监听、TUN 出站读循环与数据保留任务。

// bootAPI 启动管理 API、TUN 出站读循环与数据保留任务。
func bootAPI(bc *bootContext, sd *safeutil.Shutdown, tunDev tun.Device, tunOK, natOK bool, serverKP crypto.KeyPair) []*http.Server {
	cfg := bc.cfg
	maintenance.StartRetentionLoop(sd.Context(), bc.store, bc.logHist, cfg)
	if tunDev != nil {
		sd.Go("tun-read", func(ctx context.Context) {
			buf := make([]byte, netutil.ReadBufferSize(cfg.VPN.MTU))
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				n, err := tunDev.Read(buf)
				if err != nil {
					logger.Info("TUN 读结束: %v", err)
					return
				}
				_ = bc.sessMgr.RouteOutbound(buf[:n])
			}
		})
	}

	listenHosts := netutil.AppendTunListenHost(cfg.API.ListenHosts, tunIPString(tunDev))
	apiSrv := api.NewServer(cfg, bc.store, bc.authSvc, bc.auditLog, bc.sessMgr, bc.vpnSvc, bc.keyEnc, bc.startedAt, serverKP.PublicKey)
	apiSrv.SetLogStore(bc.logHist)
	apiSrv.SetDataplaneHealth(tunOK, natOK)
	apiSrv.SetProbeGuard(bc.probeGuard)
	apiServers := api.StartAllListeners(apiSrv, listenHosts, cfg.API.Port)
	logger.Info("管理口已就绪: %s", api.FormatBoundAddrs(apiServers))
	return apiServers
}
