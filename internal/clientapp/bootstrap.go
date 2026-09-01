package clientapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logger"
	"haovpn/internal/platform"
	"haovpn/internal/safeutil"
	"haovpn/internal/version"
)

// ErrConfigCreated 表示首次运行已生成默认 client.yaml，须用户填写后再启动。
var ErrConfigCreated = errors.New("配置已生成但未就绪")

// BootstrapMode 客户端启动入口模式（日志键 client_bootstrap mode=）。
type BootstrapMode string

const (
	BootstrapCLI     BootstrapMode = "cli"
	BootstrapService BootstrapMode = "service"
	BootstrapGUI     BootstrapMode = "gui"
)

// RunOptions 控制 CLI/服务/GUI 共用的 Engine 启动契约（预热、首连 FailFast、用户告警等）。
type RunOptions struct {
	// Mode 入口标识，仅影响日志键 client_bootstrap mode=。
	Mode BootstrapMode
	// WarmupTun 是否在 Start 前异步预热 Wintun（与鉴权重叠）。
	WarmupTun bool
	// FailFastFirst 首连鉴权失败即停重连（交互式 CLI/GUI 登录）；服务应 false。
	FailFastFirst bool
	// WaitFirstAuth 首连 WaitConnected 超时；0 表示不等待（服务长期跑）。
	WaitFirstAuth time.Duration
	// OnUserWarn ICS/部分路由等连接后用户可见告警（CLI → stderr）。
	OnUserWarn func(warn string)
	// OnDataplaneFail 鉴权成功后 TUN/路由失败（CLI → stderr 并请求退出）。
	OnDataplaneFail func(msg string)
	// WarnIfNotAdmin 日志键 cli_not_admin（须在 InitGlobal 之后生效；stderr 提示由 cmd 层负责）。
	WarnIfNotAdmin bool
}

// DefaultInteractiveRunOptions 交互式 CLI 默认：预热 + 首连 FailFast + 45s 超时 + 告警输出。
func DefaultInteractiveRunOptions() RunOptions {
	return RunOptions{
		Mode:          BootstrapCLI,
		WarmupTun:     true,
		FailFastFirst: true,
		WaitFirstAuth: DefaultFirstAuthTimeout,
		OnUserWarn: func(warn string) {
			warn = strings.TrimSpace(warn)
			if warn == "" {
				return
			}
			logger.Warn("client_user_warn chars=%d", len(warn))
			fmt.Fprintln(os.Stderr, warn)
		},
		OnDataplaneFail: func(msg string) {
			msg = strings.TrimSpace(msg)
			if msg == "" {
				msg = "配置网络失败"
			}
			fmt.Fprintln(os.Stderr, msg)
			StopCLI()
		},
		WarnIfNotAdmin: true,
	}
}

// DefaultServiceRunOptions Windows/Unix 无界面服务：预热、持续重连、不等待首连。
func DefaultServiceRunOptions() RunOptions {
	return RunOptions{
		Mode:          BootstrapService,
		WarmupTun:     true,
		FailFastFirst: false,
		WaitFirstAuth: 0,
	}
}

var cliShutdown *safeutil.Shutdown

// RunCLI 加载配置、解析凭据、启动 Engine 并阻塞直至 StopCLI 或进程信号。
//
// 使用 DefaultInteractiveRunOptions（预热、首连 FailFast、用户告警 stderr）。
func RunCLI(cfgPath string, creds Credentials) error {
	return runClient(cfgPath, creds, DefaultInteractiveRunOptions())
}

// RunServiceLoop 无界面服务主循环（FailFast 关闭、不等待首连）。
func RunServiceLoop(cfgPath string, creds Credentials) error {
	return runClient(cfgPath, creds, DefaultServiceRunOptions())
}

func runClient(cfgPath string, creds Credentials, opts RunOptions) error {
	cfg, user, password, err := loadClientBootstrap(cfgPath, creds)
	if err != nil {
		return err
	}
	defer logger.Close()

	logger.Info("HaoVPN 客户端启动 %s", version.String())
	logger.Info("client_bootstrap mode=%s warmup=%v fail_fast=%v wait_first=%s",
		opts.Mode, opts.WarmupTun, opts.FailFastFirst, opts.WaitFirstAuth)
	if opts.WarnIfNotAdmin && !platform.IsAdmin() {
		logger.Warn("cli_not_admin=true")
	}

	eng, err := startClientEngine(cfg, Credentials{Username: user, Password: password}, opts)
	if err != nil {
		return err
	}
	defer eng.Stop()

	sd := safeutil.NewShutdown()
	cliShutdown = sd
	defer func() { cliShutdown = nil }()

	logger.Info("客户端运行中…")
	<-sd.Context().Done()
	sd.Wait(10 * time.Second)
	return nil
}

// loadClientBootstrap 加载 client.yaml、初始化全局日志、解析凭据。
func loadClientBootstrap(cfgPath string, creds Credentials) (*config.ClientConfig, string, string, error) {
	cfg, created, err := config.LoadClient(cfgPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("配置错误: %w", err)
	}
	if created {
		return nil, "", "", fmt.Errorf("%w: %s（请填写 auth.username）", ErrConfigCreated, cfgPath)
	}
	if err := cfg.Log.InitGlobal(); err != nil {
		return nil, "", "", err
	}

	user := strings.TrimSpace(creds.Username)
	password := creds.Password
	if user == "" || password == "" {
		var resolveErr error
		user, password, resolveErr = ResolveCredentials(cfg)
		if resolveErr != nil {
			return nil, "", "", resolveErr
		}
	}
	if user != "" && password != "" && cfg.Auth.Password == "" {
		logger.Info("已加载登录凭据 user=%s", user)
	}
	return cfg, user, password, nil
}

// startClientEngine 按 RunOptions 预热、建 Engine、挂 hook、Start；可选等待首连。
func startClientEngine(cfg *config.ClientConfig, creds Credentials, opts RunOptions) (*Engine, error) {
	eng, err := PrepareEngine(cfg, creds, opts)
	if err != nil {
		return nil, err
	}
	if opts.WaitFirstAuth <= 0 {
		if err := eng.Start(); err != nil {
			return nil, err
		}
		return eng, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.WaitFirstAuth)
	defer cancel()
	if err := StartAndWaitFirstAuth(ctx, eng); err != nil {
		return nil, err
	}
	return eng, nil
}

// StopCLI 请求 RunCLI 优雅退出（Windows 服务 stop 时调用）。
func StopCLI() {
	if cliShutdown != nil {
		cliShutdown.Cancel()
	}
}
