package clientapp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
	"haovpn/internal/version"
)

// ErrConfigCreated 表示首次运行已生成默认 client.yaml，须用户填写后再启动。
var ErrConfigCreated = errors.New("配置已生成但未就绪")

var cliShutdown *safeutil.Shutdown

// RunCLI 加载配置、解析凭据、启动 Engine 并阻塞直至 StopCLI 或进程信号。
//
// 参数：
//   cfgPath — client.yaml 路径；
//   creds — 非空 Username/Password 时跳过 ResolveCredentials（GUI/测试注入）；
//
// 返回：ErrConfigCreated 时 cfgPath 在错误信息中；其它错误为配置/连接/日志初始化失败。
func RunCLI(cfgPath string, creds Credentials) error {
	cfg, created, err := config.LoadClient(cfgPath)
	if err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}
	if created {
		return fmt.Errorf("%w: %s（请填写 auth.username）", ErrConfigCreated, cfgPath)
	}

	if err := cfg.Log.InitGlobal(); err != nil {
		return err
	}
	defer logger.Close()
	logger.Info("HaoVPN 客户端启动 %s", version.String())

	user := strings.TrimSpace(creds.Username)
	password := creds.Password
	if user == "" || password == "" {
		var resolveErr error
		user, password, resolveErr = ResolveCredentials(cfg)
		if resolveErr != nil {
			return resolveErr
		}
	}
	if user != "" && password != "" && cfg.Auth.Password == "" {
		logger.Info("已加载登录凭据 user=%s", user)
	}

	eng := NewEngine(cfg)
	eng.SetCredentials(Credentials{
		Username: user,
		Password: password,
	})
	if err := eng.Start(); err != nil {
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

// StopCLI 请求 RunCLI 优雅退出（Windows 服务 stop 时调用）。
func StopCLI() {
	if cliShutdown != nil {
		cliShutdown.Cancel()
	}
}
