package serverapp

import (
	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/security"
	"haovpn/internal/transport"
	"haovpn/internal/tunnel"
	"haovpn/internal/tun"
)

// boot_tunnel.go：TLS 证书、隧道密钥与 ListenTLS。

// bootTunnel 加载 TLS 证书、隧道密钥并监听 TLS-TCP。
func bootTunnel(bc *bootContext, tunDev tun.Device) (*transport.Server, crypto.KeyPair, error) {
	cfg := bc.cfg
	tlsCert, err := security.LoadServerTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile, cfg.Server.TLS.AutoGenerate, &security.CertGenOptions{
		ListenAddr: cfg.Server.Listen,
		CertSANs:   cfg.Server.TLS.CertSANs,
	})
	if err != nil {
		return nil, crypto.KeyPair{}, err
	}
	tlsCfg := security.TLSConfig(tlsCert, true)

	serverKP, err := tunnel.LoadOrCreateServerKeys(bc.dataDir)
	if err != nil {
		return nil, crypto.KeyPair{}, err
	}

	tunnelHandler := &tunnel.ServerHandler{
		Store:                     bc.store,
		SessMgr:                   bc.sessMgr,
		ServerKP:                  serverKP,
		TunDev:                    tunDev,
		AllowedSourceIPs:          cfg.Security.TunnelAllowedSourceIPs,
		AllowPlaintextPrivateKeys: cfg.Security.AllowPlaintextPrivateKeys,
		Probe:                     bc.probeGuard,
		VPN:                       bc.vpnSvc,
		MTU:                       cfg.VPN.MTU,
		GatewayIP:                 cfg.VPN.GatewayIP,
		DNSServers:                cfg.VPN.DNSServers,
		VPNSubnet:                 cfg.VPN.Subnet,
		Auth:                      bc.authSvc,
		KeyEnc:                    bc.keyEnc,
	}

	tcfg := transport.FromServerVPN(cfg.VPN)
	logger.Info("transport send_queue_size=%d", tcfg.MaxQueueSize)
	// 有 Guard 即挂载：封禁表（IsBlocked）始终在 Accept 生效；Enabled 只控制自动记录/自动封。
	if bc.probeGuard != nil {
		tcfg.Probe = bc.probeGuard
	}
	tunnelSrv, err := transport.ListenTLS(cfg.Server.Listen, tlsCfg, tcfg, func(conn *transport.Conn) {
		tunnelHandler.Attach(conn)
	})
	if err != nil {
		return nil, crypto.KeyPair{}, err
	}
	logger.Info("隧道监听: %s", cfg.Server.Listen)
	return tunnelSrv, serverKP, nil
}
