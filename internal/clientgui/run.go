package clientgui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"haovpn/internal/brand"
)

// AppTheme 可选 Fyne 主题；cmd/client-gui 在 Run 前赋值为 readableTheme。
var AppTheme fyne.Theme

// Run 创建 Fyne 应用、展示登录窗并阻塞直至退出；调用方须已持有单实例锁。
func Run(configPath string, elevHint string) {
	a := app.NewWithID(brand.GUIAppID)
	if AppTheme != nil {
		a.Settings().SetTheme(AppTheme)
	}
	ui := newUI(a, configPath)
	ui.showLogin(elevHint)
	a.Run()
	ui.shutdown()
}
