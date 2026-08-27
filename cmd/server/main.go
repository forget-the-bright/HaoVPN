// cmd/server — HaoVPN 服务端入口（项目现场部署）。
//
// 加载 server.yaml（不存在则生成默认配置），委托 serverapp.Run 启动隧道与管理 API。
package main

import (
	"flag"
	"fmt"
	"os"

	"haovpn/internal/config"
	"haovpn/internal/serverapp"
	"haovpn/internal/version"
)

var (
	versionFlag = flag.Bool("version", false, "打印版本信息并退出")
	configPath  = flag.String("c", "./server.yaml", "配置文件路径")
)

func main() {
	flag.Parse()
	if *versionFlag {
		fmt.Println(version.String())
		return
	}

	cfg, created, err := config.LoadServer(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误: %v\n", err)
		os.Exit(1)
	}
	if created {
		fmt.Println("已生成默认配置，请检查后重启或继续启动:", *configPath)
	}

	if err := serverapp.New(cfg, *configPath).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "服务端退出: %v\n", err)
		os.Exit(1)
	}
}
