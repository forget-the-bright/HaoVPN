package serverapp

import (
	"context"
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
	"haovpn/internal/fileutil"
	"haovpn/internal/maintenance"
	"haovpn/internal/netstack"
	"haovpn/internal/netutil"
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

// Engine 服务端 VPN 进程的运行时入口：Run 编排数据库、隧道、TUN/NAT、管理 API 与优雅关闭。
//
// 字段：
//   cfg — 已加载的服务端配置；Run 期间只读，不在此包内修改。
//   configPath — 配置文件路径，供 health 自检与审计引用原始路径。
//
// 线程安全：Engine 本身无锁；Run 设计为单 goroutine 阻塞调用，不应并行 Run。
type Engine struct {
	cfg        *config.ServerConfig
	configPath string
}

// New 根据已加载的服务端配置构造 Engine。
func New(cfg *config.ServerConfig, configPath string) *Engine {
	return &Engine{cfg: cfg, configPath: configPath}
}

// Run 执行服务端完整启动流程，阻塞直至收到关闭信号并完成优雅 teardown。
//
// 参数：无（配置来自 New 传入的 cfg 与 configPath）。
// 返回：err — 仅日志 InitGlobal 失败时返回；其余致命错误通过 logger.Fatal 终止进程。
// 副作用：打开 DB/历史库、恢复 IP 池、监听 TLS 隧道与管理 API、启动 TUN 读与 retention goroutine；
//         关闭时依次停 API、隧道、TUN 并 Wait 子 goroutine（最多 15s）。
// 并发：内部通过 safeutil.Shutdown 管理多个 goroutine；调用方单 goroutine 阻塞直至 Run 返回。
func (e *Engine) Run() error {
	cfg := e.cfg

	// --- 阶段 1：日志与绑定安全 ---
	if err := cfg.Log.InitGlobal(); err != nil {
		return err
	}
	defer logger.Close()
	logger.Info("HaoVPN 服务端启动 %s", version.String())
	if lp := logger.LivePath(); lp != "" {
		logger.Info("同步观测日志: %s （Get-Content -Wait 可读盘，不依赖控制台）", lp)
	}

	if err := security.BindCheck(cfg.API.ListenHosts, cfg.API.AllowPublicBind); err != nil {
		logger.Fatal("管理口绑定校验失败: %v", err)
	}

	// --- 阶段 2：数据库、历史日志与认证基础 ---
	_ = fileutil.EnsureParentDir(cfg.Database.Path, 0o755)
	store, err := persist.Open(cfg.Database.Path)
	if err != nil {
		logger.Fatal("数据库打开失败: %v", err)
	}
	defer store.Close()

	var logHist *logstore.Store
	if cfg.HistoryLogEnabled() {
		hpath := cfg.ResolveHistoryDBPath()
		_ = fileutil.EnsureParentDir(hpath, 0o755)
		logHist, err = logstore.Open(hpath)
		if err != nil {
			logger.Fatal("历史日志库打开失败: %v", err)
		}
		defer logHist.Close()
		logger.SetHistoryWriter(func(level, line string) {
			logHist.Enqueue(level, line)
		})
	}

	checker := health.NewChecker(cfg, store, e.configPath)
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

	authSvc := auth.New(store, cfg.API.LoginMaxAttempts, cfg.API.LoginLockoutSec, cfg.API.SessionTTLSec)
	if err := authSvc.EnsureAdmin(cfg.Admin.Username, cfg.Admin.Password, cfg.Admin.SyncPasswordFromConfig); err != nil {
		logger.Fatal("初始化管理员失败: %v", err)
	}
	auditLog := audit.New(store)
	if cfg.API.AllowPublicBind {
		api.LogPublicBindAudit(auditLog)
	}

	// --- 阶段 3：IP 池、会话与 VPN 账号服务 ---
	ipPool, err := ippool.New(cfg.VPN.Subnet)
	if err != nil {
		logger.Fatal("IP 池初始化失败: %v", err)
	}
	ipPool.Reserve(cfg.VPN.GatewayIP)
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
		OnKickUser: func(userID int64) {
			sessMgr.KickUser(userID)
		},
	}
	sessMgr.SetDisconnectHandler(vpnSvc.ReleaseOnDisconnect)
	leaseStop := make(chan struct{})
	defer close(leaseStop)
	vpnSvc.StartLeaseCleaner(leaseStop)
	startedAt := time.Now()

	// --- 阶段 4：TLS 证书与隧道密钥 ---
	tlsCert, err := security.LoadServerTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile, cfg.Server.TLS.AutoGenerate, &security.CertGenOptions{
		ListenAddr: cfg.Server.Listen,
		CertSANs:   cfg.Server.TLS.CertSANs,
	})
	if err != nil {
		logger.Fatal("TLS 证书加载失败: %v", err)
	}
	tlsCfg := security.TLSConfig(tlsCert, true)

	serverKP, err := tunnel.LoadOrCreateServerKeys(dataDir)
	if err != nil {
		logger.Fatal("隧道密钥加载失败: %v", err)
	}

	// --- 阶段 5：TUN 与 NAT/netstack ---
	var tunDev tun.Device
	var natOK bool
	if !cfg.NAT.Enabled {
		natOK = true
	}
	gatewayCIDR := netutil.GatewayCIDR(cfg.VPN.GatewayIP, cfg.VPN.Subnet)
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

	// --- 阶段 6：隧道监听与入站连接 ---
	tcfg := transport.FromServerVPN(cfg.VPN)

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

	// --- 阶段 7：管理 API、TUN 出站读与 retention ---
	sd := safeutil.NewShutdown()
	maintenance.StartRetentionLoop(sd.Context(), store, logHist, cfg)
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
				_ = sessMgr.RouteOutbound(buf[:n])
			}
		})
	}

	listenHosts := netutil.AppendTunListenHost(cfg.API.ListenHosts, tunIPString(tunDev))
	apiSrv := api.NewServer(cfg, store, authSvc, auditLog, sessMgr, vpnSvc, keyEnc, startedAt, serverKP.PublicKey)
	apiSrv.SetLogStore(logHist)
	apiSrv.SetDataplaneHealth(tunOK, natOK)
	apiServers := api.StartAllListeners(apiSrv, listenHosts, cfg.API.Port)
	logger.Info("管理口已就绪: %s", api.FormatBoundAddrs(apiServers))

	// --- 阶段 8：等待关闭信号并优雅 teardown ---
	<-sd.Context().Done()
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
	sd.Wait(15 * time.Second)
	logger.Info("优雅关闭完成")
	return nil
}

func tunIPString(dev tun.Device) string {
	if dev == nil {
		return ""
	}
	return dev.IP().String()
}
