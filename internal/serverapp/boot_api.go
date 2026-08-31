package serverapp

import (
	"context"
	"net/http"

	"haovpn/internal/api"
	"haovpn/internal/audit"
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

	listenHosts := cfg.API.ListenHosts
	tunIP := tunIPString(tunDev)
	if cfg.API.ListenTunEnabled() {
		listenHosts = netutil.AppendTunListenHost(listenHosts, tunIP)
		if tunIP != "" {
			logger.Warn("管理口已绑定 VPN 网关 %s:%d（明文 HTTP）；VPN 内用户可访问登录页。若不需要请设 api.listen_tun: false", tunIP, cfg.API.Port)
			audit.LogTunAdminListen(bc.auditLog, tunIP, cfg.API.Port)
		}
	} else if tunIP != "" {
		logger.Info("api.listen_tun=false：管理口不绑定 VPN 网关 %s（仅 listen_hosts）", tunIP)
	}
	apiSrv := api.NewServer(cfg, bc.store, bc.authSvc, bc.auditLog, bc.sessMgr, bc.vpnSvc, bc.keyEnc, bc.startedAt, serverKP.PublicKey)
	apiSrv.SetLogStore(bc.logHist)
	apiSrv.SetDataplaneHealth(tunOK, natOK)
	apiSrv.SetProbeGuard(bc.probeGuard)
	// peerDirty 仅内存：重启后「待应用」清空；库内策略已是权威，在线客户端可能仍持旧策略直至踢线/重连。
	logger.Warn("peer 策略脏标记为进程内存态：服务重启后控制台「待应用」会清空；若曾改托管路由/互访未点应用，请检查在线客户端或手动「应用生效」")
	apiServers := api.StartAllListeners(apiSrv, listenHosts, cfg.API.Port)
	logger.Info("管理口已就绪: %s", api.FormatBoundAddrs(apiServers))
	return apiServers
}
