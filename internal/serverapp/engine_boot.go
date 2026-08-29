package serverapp

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/brand"
	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/fileutil"
	"haovpn/internal/health"
	"haovpn/internal/ippool"
	"haovpn/internal/logstore"
	"haovpn/internal/logger"
	"haovpn/internal/maintenance"
	"haovpn/internal/netstack"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
	"haovpn/internal/platform"
	"haovpn/internal/probedefense"
	"haovpn/internal/safeutil"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/transport"
	"haovpn/internal/tunnel"
	"haovpn/internal/tun"
	"haovpn/internal/vpnaccount"
)

// bootContext 服务端启动各阶段共享的运行时句柄（Run 内栈上构造，生命周期与 Run 一致）。
type bootContext struct {
	cfg        *config.ServerConfig
	configPath string
	store      *persist.Store
	logHist    *logstore.Store
	keyEnc     *security.KeyEnc
	authSvc    *auth.Service
	auditLog   *audit.Logger
	ipPool     *ippool.Pool
	sessMgr    *sessionmgr.Manager
	vpnSvc     *vpnaccount.Service
	probeGuard *probedefense.Guard
	leaseStop  chan struct{}
	startedAt  time.Time
	dataDir    string
}

// bootPersist 打开 SQLite 与可选 history 日志库，完成启动自检与管理员初始化。
func (e *Engine) bootPersist() (*bootContext, error) {
	cfg := e.cfg
	_ = fileutil.EnsureParentDir(cfg.Database.Path, 0o755)
	store, err := persist.Open(cfg.Database.Path)
	if err != nil {
		return nil, err
	}

	var logHist *logstore.Store
	if cfg.HistoryLogEnabled() {
		hpath := cfg.ResolveHistoryDBPath()
		_ = fileutil.EnsureParentDir(hpath, 0o755)
		logHist, err = logstore.Open(hpath)
		if err != nil {
			store.Close()
			return nil, err
		}
		logger.SetHistoryWriter(func(level, line string) {
			logHist.Enqueue(level, line)
		})
	}

	checker := health.NewChecker(cfg, store, e.configPath)
	if _, err := checker.RunStartupChecks(); err != nil {
		if logHist != nil {
			logHist.Close()
		}
		store.Close()
		return nil, err
	}
	if platform.IsAdmin() {
		logger.Info("自检: 当前以管理员权限运行")
	} else {
		logger.Warn("自检: 当前非管理员，TUN/NAT 可能失败（请 sudo 或「以管理员身份运行」终端）")
	}

	dataDir := filepath.Dir(cfg.Database.Path)
	keyEnc, err := security.LoadOrCreateDataKey(cfg.Database, dataDir)
	if err != nil {
		if logHist != nil {
			logHist.Close()
		}
		store.Close()
		return nil, err
	}

	authSvc := auth.New(store, cfg.API.LoginMaxAttempts, cfg.API.LoginLockoutSec, cfg.API.SessionTTLSec)
	if err := authSvc.EnsureAdmin(cfg.Admin.Username, cfg.Admin.Password, cfg.Admin.SyncPasswordFromConfig); err != nil {
		if logHist != nil {
			logHist.Close()
		}
		store.Close()
		return nil, err
	}
	auditLog := audit.New(store)
	if cfg.API.AllowPublicBind {
		api.LogPublicBindAudit(auditLog)
	}

	return &bootContext{
		cfg:        cfg,
		configPath: e.configPath,
		store:      store,
		logHist:    logHist,
		keyEnc:     keyEnc,
		authSvc:    authSvc,
		auditLog:   auditLog,
		dataDir:    dataDir,
		leaseStop:  make(chan struct{}),
		startedAt:  time.Now(),
	}, nil
}

// bootIPPool 初始化 IP 池并从 DB 恢复占用。
func bootIPPool(bc *bootContext) error {
	cfg := bc.cfg
	ipPool, err := ippool.New(cfg.VPN.Subnet)
	if err != nil {
		return err
	}
	ipPool.Reserve(cfg.VPN.GatewayIP)
	activeIPs, err := bc.store.ListActiveUserIPs()
	if err != nil {
		return err
	}
	for ip, userID := range activeIPs {
		if err := ipPool.AllocateSpecific(ip, userID); err != nil {
			return err
		}
		logger.Info("已恢复 IP 占用 ip=%s user_id=%d", ip, userID)
	}
	accounts, err := bc.store.ListVPNAccounts()
	if err != nil {
		return err
	}
	for _, u := range accounts {
		if u.VPNIP == "" || u.IPMode != persist.IPModeFixed {
			continue
		}
		if _, ok := activeIPs[u.VPNIP]; ok {
			continue
		}
		if err := ipPool.AllocateSpecific(u.VPNIP, u.ID); err != nil {
			return err
		}
		_ = bc.store.RecordIPAllocation(u.VPNIP, u.ID)
	}
	bc.ipPool = ipPool
	return nil
}

// bootSession 创建会话管理器与 VPN 账号服务。
func bootSession(bc *bootContext) error {
	sessMgr := sessionmgr.New(bc.store)
	sessMgr.SetSessionPolicy(bc.cfg.VPN.SessionPolicy)
	allowPeers := bc.cfg.Security.AllowAllVPNPeers
	if v, ok, err := bc.store.GetAllowAllVPNPeersSetting(); err == nil && ok {
		allowPeers = v
		bc.cfg.Security.AllowAllVPNPeers = v
	}
	sessMgr.SetAllowAllVPNPeers(allowPeers)
	if bc.cfg.VPN.ReconnectGraceSec > 0 {
		sessMgr.SetReconnectGrace(time.Duration(bc.cfg.VPN.ReconnectGraceSec) * time.Second)
	}
	if err := sessMgr.LoadVPNIPIndex(); err != nil {
		return err
	}
	vpnSvc := &vpnaccount.Service{
		Store: bc.store,
		Pool:  bc.ipPool,
		Cfg:   bc.cfg,
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
	sessMgr.SetDisconnectHandler(func(userID int64, vpnIP, ipMode string) {
		if err := bc.store.ClearClientLANRegistry(userID); err != nil {
			logger.Warn("断线清 lan_registry 失败 user_id=%d: %v", userID, err)
		}
		vpnSvc.ReleaseOnDisconnect(userID, vpnIP, ipMode)
	})
	vpnSvc.StartLeaseCleaner(bc.leaseStop)
	bc.sessMgr = sessMgr
	bc.vpnSvc = vpnSvc
	bc.probeGuard = probedefense.New(bc.store, probedefense.ConfigFromServer(bc.cfg.Security))
	if bc.probeGuard.Enabled() {
		logger.Info("探针防御已启用 auto_ban=%v ban_after=%d window=%ds",
			bc.cfg.Security.ProbeDefense.IsAutoBan(),
			bc.cfg.Security.ProbeDefense.BanAfterEvents,
			bc.cfg.Security.ProbeDefense.BanWindowSec)
	}
	return nil
}

type tunNetstackResult struct {
	tunDev tun.Device
	ns     *netstack.Stack
	tunOK  bool
	natOK  bool
}

// bootTUNNetstack 创建 TUN 并配置路由/NAT。
func bootTUNNetstack(bc *bootContext) (*tunNetstackResult, error) {
	cfg := bc.cfg
	res := &tunNetstackResult{natOK: !cfg.NAT.Enabled}
	gatewayCIDR := netutil.GatewayCIDR(cfg.VPN.GatewayIP, cfg.VPN.Subnet)
	tunDev, err := tun.Open(tun.Config{Name: brand.DefaultTunName, MTU: cfg.VPN.MTU, CIDR: gatewayCIDR})
	if err != nil {
		if cfg.VPN.RequireTun {
			return nil, err
		}
		logger.Warn("TUN 创建失败（需管理员权限：sudo 或提权终端）: %v", err)
		return res, nil
	}
	res.tunDev = tunDev
	res.tunOK = true

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
			_ = tunDev.Close()
			return nil, err
		}
		logger.Error("netstack Setup 失败: %v", err)
	} else if ns.SNATEnabled() {
		res.natOK = true
	}
	res.ns = ns
	return res, nil
}

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
