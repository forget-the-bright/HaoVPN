// cmd/client-gui — HaoVPN 跨平台桌面客户端入口（Fyne：登录 / 日志 / 托盘）。
//
// Windows 非管理员时尝试 UAC 提权；单实例锁后委托 clientgui.Run。
// 亦支持 Windows 服务入口（args「service」/「--service」），与 CLI 共用 clientapp。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"haovpn/internal/clientapp"
	"haovpn/internal/clientgui"
	"haovpn/internal/config"
	"haovpn/internal/platform"
	"haovpn/internal/singleinstance"
	"haovpn/internal/version"
)

func main() {
	// 服务 / --service：无 Fyne，仅 Engine（托盘「服务开机自启」注册本 exe）
	if clientapp.RunServiceCommand(os.Args) {
		return
	}

	configPathFlag := flag.String("c", "", "配置文件路径（默认：exe 同目录 client.yaml）")
	versionFlag := flag.Bool("version", false, "打印版本并退出")
	flag.Parse()
	if *versionFlag {
		fmt.Println(version.String())
		return
	}

	configPath := strings.TrimSpace(*configPathFlag)
	if configPath == "" {
		configPath = config.ResolveClientConfigPath()
	}

	clientgui.AppTheme = readableTheme{}

	// 协调口已被占用：若是本产品服务则询问接管，否则提示已在运行。
	if singleinstance.ClientAlreadyRunning() {
		if handleOccupiedInstance() {
			// 已接管，继续往下抢锁
		} else {
			return // 已提示退出或保持服务退出
		}
	}

	elevHint := ""
	if !platform.IsAdmin() {
		launched, err := platform.RelaunchElevated()
		if launched {
			os.Exit(0)
		}
		if err != nil {
			elevHint = "须以管理员运行（TUN/路由/杀开关）。提权失败：" + err.Error()
		} else {
			elevHint = "须以管理员运行（TUN/路由/杀开关）"
		}
	}

	if singleinstance.ClientAlreadyRunning() {
		if handleOccupiedInstance() {
			// continue
		} else {
			return
		}
	}

	clientLock, err := singleinstance.AcquireClient()
	if err != nil {
		msg := singleinstance.AlreadyRunningMessage()
		if !errors.Is(err, singleinstance.ErrAlreadyRunning) {
			msg = "无法获取单实例锁: " + err.Error()
		}
		if errors.Is(err, singleinstance.ErrAlreadyRunning) {
			if handleOccupiedInstance() {
				clientLock, err = singleinstance.AcquireClient()
			}
			if err != nil {
				clientgui.ShowAlreadyRunning(msg)
				return
			}
		} else {
			clientgui.ShowFatalNotice("无法启动", msg)
			return
		}
	}
	defer clientLock.Release()

	clientgui.Run(configPath, elevHint)
}

// handleOccupiedInstance 处理协调口已被占用。
//
// 返回 true：已停止服务并应继续 Acquire；false：已向用户说明并应结束 main。
func handleOccupiedInstance() bool {
	_, running, _, err := clientapp.ServiceAutostartStatus()
	if err == nil && running {
		act := clientgui.AskServiceTakeover()
		if act == clientgui.ServiceTakeoverKeep {
			os.Exit(0)
			return false
		}
		if err := clientgui.StopServiceForTakeover(); err != nil {
			clientgui.ShowFatalNotice("接管失败", "停止服务失败: "+err.Error())
			return false
		}
		if !clientgui.WaitSingleInstanceFree(5 * time.Second) {
			clientgui.ShowFatalNotice("接管失败", "服务已停但仍无法获取单实例锁，请稍后重试")
			return false
		}
		return true
	}
	clientgui.ShowAlreadyRunning(clientapp.SingleInstanceUserMessage(false))
	return false
}
