// cmd/server 是 HaoVPN 服务端入口（项目现场部署）。
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/brand"
	"haovpn/internal/config"
	"haovpn/internal/health"
	"haovpn/internal/ippool"
	"haovpn/internal/logstore"
	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/persist"
	"haovpn/internal/platform"
	"haovpn/internal/safeutil"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/transport"
	"haovpn/internal/tunnel"
	"haovpn/internal/tun"
	"haovpn/internal/version"
	"haovpn/internal/vpnaccount"
)

var (
	versionFlag = flag.Bool("version", false, "打印版本信息并退出")
	configPath  = flag.String("c", "./server.yaml", "配置文件路径")
)

func main() {
	flag.Parse()
	if *versionFlag {
		fmt.Println(version.String())
		return
	}

	cfg, created, err := config.LoadServer(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误: %v\n", err)
		os.Exit(1)
	}
	if created {
		fmt.Println("已生成默认配置，请检查后重启或继续启动:", *configPath)
	}

	if err := logger.Init(logger.Config{
		Level:      cfg.Log.Level,
		File:       cfg.Log.File,
		MaxSizeMB:  cfg.Log.MaxSizeMB,
		MaxBackups: cfg.Log.MaxBackups,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "日志初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()
	logger.Info("HaoVPN 服务端启动 %s", version.String())
	if lp := logger.LivePath(); lp != "" {
		logger.Info("同步观测日志: %s （Get-Content -Wait 可读盘，不依赖控制台）", lp)
	}

	if err := security.BindCheck(cfg.API.ListenHosts, cfg.API.AllowPublicBind); err != nil {
		logger.Fatal("管理口绑定校验失败: %v", err)
	}

	_ = os.MkdirAll(filepath.Dir(cfg.Database.Path), 0o755)
	store, err := persist.Open(cfg.Database.Path)
	if err != nil {
		logger.Fatal("数据库打开失败: %v", err)
	}
	defer store.Close()

	var logHist *logstore.Store
	if cfg.HistoryLogEnabled() {
		hpath := cfg.ResolveHistoryDBPath()
		_ = os.MkdirAll(filepath.Dir(hpath), 0o755)
		logHist, err = logstore.Open(hpath)
		if err != nil {
			logger.Fatal("历史日志库打开失败: %v", err)
		}
		defer logHist.Close()
		logger.SetHistoryWriter(func(level, line string) {
			logHist.Enqueue(level, line)
		})
	}

	checker := health.NewChecker(cfg, store, *configPath)
	if _, err := checker.RunStartupChecks(); err != nil {
		logger.Fatal("启动自检失败: %v", err)
	}
	if platform.IsAdmin() {
		logger.Info("自检: 当前以管理员权限运行")
	} else {
		logger.Warn("自检: 当前非管理员，TUN/NAT 可能失败（请 sudo 或「以管理员身份运行」终端）")
	}

	dataDir := filepath.Dir(cfg.Database.Path)
	keyEnc, err := security.LoadOrCreateDataKey(cfg.Database, dataDir)
	if err != nil {
		logger.Fatal("数据库加密密钥加载失败: %v", err)
	}
	if err := store.MigratePlaintextPrivateKeys(keyEnc); err != nil {
		logger.Fatal("私钥加密迁移失败: %v", err)
	}

	authSvc := auth.New(store, cfg.API.LoginMaxAttempts, cfg.API.LoginLockoutSec, cfg.API.SessionTTLSec)
	if err := authSvc.EnsureAdmin(cfg.Admin.Username, cfg.Admin.Password, cfg.Admin.SyncPasswordFromConfig); err != nil {
		logger.Fatal("初始化管理员失败: %v", err)
	}
	auditLog := audit.New(store)
	if cfg.API.AllowPublicBind {
		api.LogPublicBindAudit(auditLog)
	}

	ipPool, err := ippool.New(cfg.VPN.Subnet)
	if err != nil {
		logger.Fatal("IP 池初始化失败: %v", err)
	}
	ipPool.Reserve(cfg.VPN.GatewayIP)
	// 恢复全部未释放占用（含 dynamic_lease 租约期内），避免重启后撞车。
	activeIPs, err := store.ListActiveUserIPs()
	if err != nil {
		logger.Fatal("加载 IP 占用失败: %v", err)
	}
	for ip, userID := range activeIPs {
		if err := ipPool.AllocateSpecific(ip, userID); err != nil {
			logger.Fatal("恢复 IP 占用失败 ip=%s user=%d: %v", ip, userID, err)
		}
		logger.Info("已恢复 IP 占用 ip=%s user_id=%d", ip, userID)
	}
	accounts, err := store.ListVPNAccounts()
	if err != nil {
		logger.Fatal("加载 VPN 账号列表失败: %v", err)
	}
	for _, u := range accounts {
		if u.VPNIP == "" || u.IPMode != persist.IPModeFixed {
			continue
		}
		if _, ok := activeIPs[u.VPNIP]; ok {
			continue
		}
		if err := ipPool.AllocateSpecific(u.VPNIP, u.ID); err != nil {
			logger.Fatal("恢复固定 IP 失败 user %d ip %s: %v", u.ID, u.VPNIP, err)
		}
		_ = store.RecordIPAllocation(u.VPNIP, u.ID)
	}

	sessMgr := sessionmgr.New(store)
	if err := sessMgr.LoadVPNIPIndex(); err != nil {
		logger.Fatal("加载 VPN IP 索引失败: %v", err)
	}

	vpnSvc := &vpnaccount.Service{
		Store: store,
		Pool:  ipPool,
		Cfg:   cfg,
		OnRegisterIP: func(vpnIP string, userID int64) {
			sessMgr.RegisterVPNIP(vpnIP, userID)
		},
		OnUnregisterIP: func(vpnIP string) {
			sessMgr.UnregisterVPNIP(vpnIP)
		},
	}
	sessMgr.SetDisconnectHandler(vpnSvc.ReleaseOnDisconnect)
	leaseStop := make(chan struct{})
	defer close(leaseStop)
	vpnSvc.StartLeaseCleaner(leaseStop)
	startedAt := time.Now()

	tlsCert, err := security.LoadServerTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile, cfg.Server.TLS.AutoGenerate, &security.CertGenOptions{
		ListenAddr: cfg.Server.Listen,
		CertSANs:   cfg.Server.TLS.CertSANs,
	})
	if err != nil {
		logger.Fatal("TLS 证书加载失败: %v", err)
	}
	tlsCfg := security.TLSConfig(tlsCert, true)

	// 持久化隧道密钥（与 TLS 证书分离）
	serverKP, err := tunnel.LoadOrCreateServerKeys(dataDir)
	if err != nil {
		logger.Fatal("隧道密钥加载失败: %v", err)
	}

	var tunDev tun.Device
	var natOK bool
	if !cfg.NAT.Enabled {
		natOK = true
	}
	gatewayCIDR := cfg.VPN.GatewayIP + "/" + maskFromSubnet(cfg.VPN.Subnet)
	tunDev, err = tun.Open(tun.Config{Name: brand.DefaultTunName, MTU: cfg.VPN.MTU, CIDR: gatewayCIDR})
	if err != nil {
		if cfg.VPN.RequireTun {
			logger.Fatal("TUN 创建失败（需管理员权限：sudo 或提权终端）: %v", err)
		}
		logger.Warn("TUN 创建失败（需管理员权限：sudo 或提权终端）: %v", err)
	} else {
		ns := netstack.New(netstack.Config{
			TunName:     tunDev.Name(),
			TunIP:       tunDev.IP(),
			VPNSubnet:   cfg.VPN.Subnet,
			LanCIDRs:    cfg.NAT.AllowedLANCIDRs,
			OutboundIf:  cfg.NAT.OutboundInterface,
			ForwardOnly: cfg.NAT.ForwardOnly,
			Enabled:     cfg.NAT.Enabled,
		})
		if err := ns.Setup(); err != nil {
			if cfg.NAT.Enabled {
				logger.Fatal("netstack Setup 失败（nat.enabled=true）: %v", err)
			}
			logger.Error("netstack Setup 失败: %v", err)
		} else if ns.SNATEnabled() {
			natOK = true
		}
		defer func() { _ = ns.Teardown() }()
		defer func() { _ = tunDev.Close() }()
	}
	tunOK := tunDev != nil

	tcfg := transport.DefaultConfig()
	if cfg.VPN.HeartbeatIntervalSec > 0 {
		tcfg.HeartbeatInterval = time.Duration(cfg.VPN.HeartbeatIntervalSec) * time.Second
	}
	tcfg.HeartbeatTimeout = time.Duration(cfg.VPN.HeartbeatTimeoutSec) * time.Second
	if tcfg.HeartbeatTimeout <= 0 {
		tcfg.HeartbeatTimeout = 90 * time.Second
	}

	tunnelHandler := &tunnel.ServerHandler{
		Store:            store,
		SessMgr:          sessMgr,
		ServerKP:         serverKP,
		TunDev:           tunDev,
		AllowedSourceIPs: cfg.Security.TunnelAllowedSourceIPs,
		VPN:              vpnSvc,
		MTU:              cfg.VPN.MTU,
		GatewayIP:        cfg.VPN.GatewayIP,
		DNSServers:       cfg.VPN.DNSServers,
		Auth:             authSvc,
		KeyEnc:           keyEnc,
	}

	tunnelSrv, err := transport.ListenTLS(cfg.Server.Listen, tlsCfg, tcfg, func(conn *transport.Conn) {
		tunnelHandler.Attach(conn)
	})
	if err != nil {
		logger.Fatal("隧道监听失败: %v", err)
	}
	logger.Info("隧道监听: %s", cfg.Server.Listen)

	sd := safeutil.NewShutdown()
	api.StartRetentionLoop(sd.Context(), store, logHist, cfg)
	if tunDev != nil {
		sd.Go("tun-read", func(ctx context.Context) {
			buf := make([]byte, cfg.VPN.MTU+100)
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
				_ = sessMgr.RouteOutbound(buf[:n])
			}
		})
	}

	listenHosts := api.AppendTunListenHost(cfg.API.ListenHosts, tunIPString(tunDev))
	apiSrv := api.NewServer(cfg, store, authSvc, auditLog, sessMgr, ipPool, keyEnc, startedAt, serverKP.PublicKey)
	apiSrv.SetLogStore(logHist)
	apiSrv.SetVPNService(vpnSvc)
	apiSrv.SetDataplaneHealth(tunOK, natOK)
	apiServers := api.StartAllListeners(apiSrv, listenHosts, cfg.API.Port)
	logger.Info("管理口已就绪: %s", api.FormatBoundAddrs(apiServers))

	<-sd.Context().Done()
	logger.Info("正在优雅关闭...")
	for _, srv := range apiServers {
		_ = srv.Close()
	}
	if tunnelSrv != nil {
		_ = tunnelSrv.Close()
	}
	if tunDev != nil {
		// 先关 TUN 唤醒 tun-read，再等待协程退出，避免 defer Close 与 ReceivePacket 竞态崩溃
		_ = tunDev.Close()
	}
	sd.Wait(15 * time.Second)
	logger.Info("优雅关闭完成")
}

func tunIPString(dev tun.Device) string {
	if dev == nil {
		return ""
	}
	return dev.IP().String()
}

func maskFromSubnet(subnet string) string {
	_, n, err := net.ParseCIDR(subnet)
	if err != nil {
		return "24"
	}
	ones, _ := n.Mask.Size()
	return fmt.Sprintf("%d", ones)
}
