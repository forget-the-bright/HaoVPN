package serverapp

import (
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
	"haovpn/internal/security"
	"haovpn/internal/version"
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

// Run 执行服务端完整启动流程，阻塞直至收到关闭信号并完成优雅 teardown。//
// 参数：无（配置来自 New 传入的 cfg 与 configPath）。
// 返回：err — 仅日志 InitGlobal 失败时返回；其余致命错误通过 logger.Fatal 终止进程。
// 副作用：打开 DB/历史库、恢复 IP 池、监听 TLS 隧道与管理 API、启动 TUN 读与 retention goroutine；
//         关闭时依次停 API、隧道、TUN 并 Wait 子 goroutine（最多 15s）。
// 并发：内部通过 safeutil.Shutdown 管理多个 goroutine；调用方单 goroutine 阻塞直至 Run 返回。
func (e *Engine) Run() error {
	cfg := e.cfg

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

	bc, err := e.bootPersist()
	if err != nil {
		logger.Fatal("持久化层启动失败: %v", err)
	}
	defer bc.store.Close()
	if bc.logHist != nil {
		defer bc.logHist.Close()
	}
	defer close(bc.leaseStop)

	if err := bootIPPool(bc); err != nil {
		logger.Fatal("IP 池初始化失败: %v", err)
	}
	if err := bootSession(bc); err != nil {
		logger.Fatal("会话层启动失败: %v", err)
	}

	tns, err := bootTUNNetstack(bc)
	if err != nil {
		if cfg.VPN.RequireTun {
			logger.Fatal("TUN 创建失败（需管理员权限：sudo 或提权终端）: %v", err)
		}
		if cfg.NAT.Enabled {
			logger.Fatal("netstack Setup 失败（nat.enabled=true）: %v", err)
		}
		logger.Fatal("TUN/netstack 启动失败: %v", err)
	}
	if tns.ns != nil {
		defer func() { _ = tns.ns.Teardown() }()
	}
	if tns.tunDev != nil {
		defer func() { _ = tns.tunDev.Close() }()
	}

	tunnelSrv, serverKP, err := bootTunnel(bc, tns.tunDev)
	if err != nil {
		logger.Fatal("隧道监听失败: %v", err)
	}

	sd := safeutil.NewShutdown()
	apiServers := bootAPI(bc, sd, tns.tunDev, tns.tunOK, tns.natOK, serverKP)

	<-sd.Context().Done()
	shutdownServer(sd, apiServers, tunnelSrv, tns.tunDev, 15*time.Second)
	return nil
}
