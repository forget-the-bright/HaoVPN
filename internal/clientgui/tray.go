package clientgui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"

	"haovpn/internal/brand"
	"haovpn/internal/clientapp"
)

// installTray 应用启动后安装系统托盘（登录前即存在）。
func (u *uiApp) installTray() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	desk.SetSystemTrayIcon(theme.ComputerIcon())
	u.refreshTrayMenu()
}

// refreshTrayMenu 按是否已连接 VPN 刷新托盘菜单项。
func (u *uiApp) refreshTrayMenu() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	if u.eng != nil {
		desk.SetSystemTrayMenu(u.mainTrayMenu())
	} else {
		desk.SetSystemTrayMenu(u.loginTrayMenu())
	}
}

func (u *uiApp) loginTrayMenu() *fyne.Menu {
	return fyne.NewMenu(brand.Name,
		fyne.NewMenuItem("显示登录窗口", func() { u.showLoginWindow() }),
		fyne.NewMenuItem("退出", func() { u.quitApp() }),
	)
}

func (u *uiApp) mainTrayMenu() *fyne.Menu {
	return fyne.NewMenu(brand.Name,
		fyne.NewMenuItem("显示主窗口", func() { u.showMainWindow() }),
		fyne.NewMenuItem("重新连接", func() { u.reconnectVPN() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("退出登录", func() { u.doLogout() }),
		fyne.NewMenuItem("退出", func() { u.quitApp() }),
	)
}

// showLoginWindow 从托盘恢复登录窗。
func (u *uiApp) showLoginWindow() {
	if u.loginWin == nil {
		u.showLogin("")
		return
	}
	u.loginWin.Show()
	u.loginWin.CenterOnScreen()
}

// showMainWindow 从托盘恢复主窗口。
func (u *uiApp) showMainWindow() {
	if u.mainWin == nil {
		return
	}
	u.mainWin.Show()
	u.mainWin.CenterOnScreen()
}

// reconnectVPN 手动重新拨号（托盘与主窗共用）。
func (u *uiApp) reconnectVPN() {
	if u.eng == nil {
		return
	}
	creds := clientapp.Credentials{
		Username: strings.TrimSpace(u.userEntry.Text),
		Password: u.passEntry.Text,
	}
	u.eng.Stop()
	u.eng = clientapp.NewEngine(u.cfg)
	u.eng.SetCredentials(creds)
	_ = u.eng.Start()
	u.appendLog("手动重新连接…")
}

// quitApp 托盘/主窗「退出」：清理后结束进程。
func (u *uiApp) quitApp() {
	u.shutdown()
	u.app.Quit()
}
