// cmd/client-gui — HaoVPN 跨平台桌面客户端入口（Fyne：登录 / 日志 / 托盘）。
//
// Windows 非管理员时尝试 UAC 提权；单实例锁后委托 clientgui.Run。
package main



import (

	"errors"

	"flag"

	"fmt"

	"strings"



	"fyne.io/fyne/v2"

	"fyne.io/fyne/v2/app"

	"fyne.io/fyne/v2/dialog"



	"haovpn/internal/brand"

	"haovpn/internal/clientgui"

	"haovpn/internal/config"

	"haovpn/internal/platform"

	"haovpn/internal/singleinstance"

	"haovpn/internal/version"

)



func main() {

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



	// Windows：非管理员则 UAC 提权重启；用户拒绝后仍进 GUI，登录窗提示须管理员。

	elevHint := ""

	if !platform.IsAdmin() {

		launched, err := platform.RelaunchElevated()

		if launched {

			return

		}

		if err != nil {

			elevHint = "须以管理员运行（TUN/路由/杀开关）。提权失败：" + err.Error()

		} else {

			elevHint = "须以管理员运行（TUN/路由/杀开关）"

		}

	}



	clientLock, err := singleinstance.AcquireClient()

	if err != nil {

		a := app.NewWithID(brand.GUIAppID)

		w := a.NewWindow(brand.Name)

		w.Resize(fyne.NewSize(480, 160))

		msg := singleinstance.AlreadyRunningMessage()

		if !errors.Is(err, singleinstance.ErrAlreadyRunning) {

			msg = "无法获取单实例锁: " + err.Error()

		}

		dialog.ShowInformation("无法启动", msg, w)

		w.Show()

		a.Run()

		return

	}

	defer clientLock.Release()



	clientgui.AppTheme = readableTheme{}

	clientgui.Run(configPath, elevHint)

}


