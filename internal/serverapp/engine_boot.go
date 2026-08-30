package serverapp

import (
	"time"

	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/ippool"
	"haovpn/internal/logstore"
	"haovpn/internal/netstack"
	"haovpn/internal/persist"
	"haovpn/internal/probedefense"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/tun"
	"haovpn/internal/vpnaccount"
)

// engine_boot.go：启动阶段共享的 bootContext 与 TUN/NAT 结果结构。
// 各 boot* 实现拆到 boot_persist / boot_ippool / boot_session / boot_tun / boot_tunnel / boot_api。

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

type tunNetstackResult struct {
	tunDev tun.Device
	ns     *netstack.Stack
	tunOK  bool
	natOK  bool
}
