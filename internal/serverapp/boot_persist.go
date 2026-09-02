package serverapp

import (
	"path/filepath"
	"time"

	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/fileutil"
	"haovpn/internal/health"
	"haovpn/internal/logger"
	"haovpn/internal/logstore"
	"haovpn/internal/persist"
	"haovpn/internal/platform"
	"haovpn/internal/safeutil"
	"haovpn/internal/security"
)

// boot_persist.go：打开 SQLite / history、自检、管理员与 auth 清理。

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
	leaseStop := make(chan struct{})
	// 后台定期清理过期 Web 会话与锁定表，避免长跑进程内存增长
	safeutil.GoSafe("auth-prune", func() {
		safeutil.RunTickerStop(leaseStop, 5*time.Minute, func() {
			ns := authSvc.PruneExpiredSessions()
			nl := authSvc.PruneExpiredLockouts()
			if ns > 0 || nl > 0 {
				logger.Info("auth_prune sessions=%d lockouts=%d", ns, nl)
			}
		})
	})
	auditLog := audit.New(store)
	if cfg.API.AllowPublicBind {
		audit.LogPublicBindEnabled(auditLog)
	}

	// 托管 DNS：YAML vpn.dns_servers → source=config + members=all（保留 excludes）
	if added, kept, removed, err := store.SyncConfigDNSServers(cfg.VPN.DNSServers); err != nil {
		logger.Warn("托管 DNS YAML seed 失败: %v", err)
	} else {
		logger.Info("dns_seed yaml_count=%d added=%d kept=%d removed=%d", len(cfg.VPN.DNSServers), added, kept, removed)
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
		leaseStop:  leaseStop,
		startedAt:  time.Now(),
	}, nil
}
