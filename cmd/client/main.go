// cmd/client 是 HaoVPN 客户端入口（工程师侧 CLI 拨号）。
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"haovpn/internal/clientapp"
	"haovpn/internal/config"
	"haovpn/internal/credentials"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
	"haovpn/internal/version"
)

var (
	versionFlag = flag.Bool("version", false, "打印版本信息并退出")
	configPath  = flag.String("c", "./client.yaml", "配置文件路径")
	clientSD    *safeutil.Shutdown
)

func main() {
	flag.Parse()
	if *versionFlag {
		fmt.Println(version.String())
		return
	}
	if runServiceCommand() {
		return
	}

	if err := runClient(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "客户端退出: %v\n", err)
		os.Exit(1)
	}
}

func runClient(cfgPath string) error {
	cfg, created, err := config.LoadClient(cfgPath)
	if err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}
	if created {
		fmt.Println("已生成默认客户端配置:", cfgPath)
		return fmt.Errorf("配置已生成但未就绪: %s（请填写 auth.username）", cfgPath)
	}

	if err := logger.Init(logger.Config{
		Level:      cfg.Log.Level,
		File:       cfg.Log.File,
		MaxSizeMB:  cfg.Log.MaxSizeMB,
		MaxBackups: cfg.Log.MaxBackups,
	}); err != nil {
		return fmt.Errorf("日志初始化失败: %w", err)
	}
	defer logger.Close()
	logger.Info("HaoVPN 客户端启动 %s", version.String())

	user, password := cfg.ResolveAuth()
	if user == "" {
		// Windows 服务自启：尝试 DPAPI 凭据
		cu, cp, err := credentials.LoadService()
		if err != nil {
			logger.Error("读取服务凭据失败: %v", err)
			return err
		}
		if cu != "" && cp != "" {
			user, password = cu, cp
			logger.Info("已加载 Windows 服务凭据 user=%s", user)
		}
	}
	if user == "" {
		return fmt.Errorf("请配置 auth.username 或保存服务凭据")
	}
	if password == "" {
		fmt.Fprint(os.Stderr, "请输入密码（或设置 HAOVPN_PASSWORD）: ")
		var p string
		if _, err := fmt.Scanln(&p); err != nil || p == "" {
			return fmt.Errorf("需要密码才能连接")
		}
		password = p
	}

	eng := clientapp.NewEngine(cfg)
	eng.SetCredentials(clientapp.Credentials{
		Username: user,
		Password: password,
	})
	if err := eng.Start(); err != nil {
		return err
	}
	defer eng.Stop()

	sd := safeutil.NewShutdown()
	clientSD = sd
	defer func() { clientSD = nil }()

	logger.Info("客户端运行中…")
	<-sd.Context().Done()
	sd.Wait(10 * time.Second)
	return nil
}

func stopClient() {
	if clientSD != nil {
		clientSD.Cancel()
	}
}
