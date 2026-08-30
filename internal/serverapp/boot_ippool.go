package serverapp

import (
	"haovpn/internal/ippool"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// boot_ippool.go：VPN IP 池初始化与 DB 占用恢复。

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
