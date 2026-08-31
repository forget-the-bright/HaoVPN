package clientgui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"haovpn/internal/brand"
	"haovpn/internal/clientapp"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
)

// AppTheme 可选 Fyne 主题；cmd/client-gui 在 Run 前赋值为 readableTheme。
var AppTheme fyne.Theme

// Run 创建 Fyne 应用、安装托盘并阻塞直至退出；调用方须已持有单实例锁。
//
// elevHint 非空时在登录窗提示须管理员。start_minimized / auto_connect 见 client.yaml gui 段。
func Run(configPath string, elevHint string) {
	// 须在 NewWithID 之前：纯 go build 不加载 FyneApp.toml Migrations。
	applyFyneDoMigration()
	a := app.NewWithID(brand.GUIAppID)
	if AppTheme != nil {
		a.Settings().SetTheme(AppTheme)
	}
	ui := newUI(a, configPath)
	ui.ensureConfigLoaded()
	ui.installTray()

	// 后台预热 Wintun（与拨号/鉴权重叠）；勿 Wait 后再 auto_connect，否则 UI 空等数秒。
	if ui.cfg != nil {
		tunName := ui.cfg.Tun.Name
		safeutil.GoSafe("gui-tun-warmup", func() {
			// 经 clientapp 门面预热，禁止 GUI 直接 import tun（分层）。
			if err := clientapp.WarmupTun(tunName); err != nil {
				logger.Warn("tun_warmup fail name=%s: %v（登录时仍会 Open/Create）", tunName, err)
			}
		})
	}

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
			logger.Info("gui_auto_connect begin warmup_overlap=true")
			fyne.Do(func() { ui.maybeAutoConnect() })
		})
	}

	a.Run()
	ui.shutdown()
}
