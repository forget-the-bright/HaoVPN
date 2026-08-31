package serverapp

import (
	"haovpn/internal/logger"
	"haovpn/internal/probedefense"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/timeutil"
	"haovpn/internal/vpnaccount"
)

// boot_session.go：会话管理器、VPN 账号服务与探针防御。

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
		sessMgr.SetReconnectGrace(timeutil.Seconds(bc.cfg.VPN.ReconnectGraceSec))
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
	if err := probedefense.ImportBanExemptFromYAML(bc.store, bc.cfg.Security.ProbeDefense.BanExemptIPs); err != nil {
		return err
	}
	if err := bc.probeGuard.ReloadBanExempt(); err != nil {
		return err
	}
	if bc.probeGuard.Enabled() {
		logger.Info("探针防御已启用 auto_ban=%v ban_after=%d window=%ds",
			bc.cfg.Security.ProbeDefense.IsAutoBan(),
			bc.cfg.Security.ProbeDefense.BanAfterEvents,
			bc.cfg.Security.ProbeDefense.BanWindowSec)
	}
	return nil
}
