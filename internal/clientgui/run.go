package clientgui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
)

// AppTheme 可选 Fyne 主题；cmd/client-gui 在 Run 前赋值为 readableTheme。
var AppTheme fyne.Theme

// Run 创建 Fyne 应用、安装托盘并阻塞直至退出；调用方须已持有单实例锁。
//
// elevHint 非空时在登录窗提示须管理员。start_minimized / auto_connect 见 client.yaml gui 段。
func Run(configPath string, elevHint string) {
	a := app.NewWithID(brand.GUIAppID)
	if AppTheme != nil {
		a.Settings().SetTheme(AppTheme)
	}
	ui := newUI(a, configPath)
	ui.ensureConfigLoaded()
	ui.installTray()

	minimized := ui.cfg != nil && ui.cfg.GUI.StartMinimized
	if minimized {
		logger.Info("gui_start_minimized=true（仅托盘）")
		// 仍构建登录窗但不 Show，便于自动连接填表与托盘「显示登录窗口」
		ui.showLogin(elevHint)
		if ui.loginWin != nil {
			ui.loginWin.Hide()
		}
	} else {
		ui.showLogin(elevHint)
	}

	if ui.cfg != nil && ui.cfg.CanAutoConnect() {
		safeutil.GoSafe("gui-auto-connect", func() {
			time.Sleep(300 * time.Millisecond)
			fyne.Do(func() { ui.maybeAutoConnect() })
		})
	}

	a.Run()
	ui.shutdown()
}
