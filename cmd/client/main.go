// cmd/client — HaoVPN CLI 客户端入口（工程师侧拨号）。
//
// 加载 client.yaml、单实例锁后委托 clientapp.RunCLI；支持 -version 与 Windows 服务子命令。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"haovpn/internal/clientapp"
	"haovpn/internal/config"
	"haovpn/internal/platform"
	"haovpn/internal/singleinstance"
	"haovpn/internal/version"
)

func main() {
	versionFlag := flag.Bool("version", false, "打印版本信息并退出")
	configPath := flag.String("c", "", "配置文件路径（默认：exe 同目录 client.yaml）")
	flag.Parse()
	if *versionFlag {
		fmt.Println(version.String())
		return
	}
	if runServiceCommand() {
		return
	}

	if !platform.IsAdmin() {
		fmt.Fprintln(os.Stderr, "警告：当前未以管理员运行，TUN/路由/杀开关可能失败。请以管理员身份运行 haovpn-client。")
	}

	clientLock, err := singleinstance.AcquireClient()
	if err != nil {
		if errors.Is(err, singleinstance.ErrAlreadyRunning) {
			_, serviceRunning, _, _ := clientapp.ServiceAutostartStatus()
			fmt.Fprintln(os.Stderr, clientapp.SingleInstanceHint(serviceRunning))
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "单实例锁失败: %v\n", err)
		os.Exit(1)
	}
	defer clientLock.Release()

	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = config.ResolveClientConfigPath()
	}
	if err := clientapp.RunCLI(cfgPath, clientapp.Credentials{}); err != nil {
		if errors.Is(err, clientapp.ErrConfigCreated) {
			fmt.Println("已生成默认客户端配置:", cfgPath)
		}
		fmt.Fprintf(os.Stderr, "客户端退出: %v\n", err)
		os.Exit(1)
	}
}
