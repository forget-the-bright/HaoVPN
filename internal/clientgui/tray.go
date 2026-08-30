package clientgui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"haovpn/internal/brand"
	"haovpn/internal/clientapp"
	"haovpn/internal/clientgui/icons"
	"haovpn/internal/logger"
)

// trayKind 托盘图标种类（避免无意义重设；SetSystemTrayMenu 会冲掉图标，须随后再设）。
type trayKind int

const (
	trayKindIdle trayKind = iota
	trayKindConnecting
	trayKindConnected
	trayKindError
)

// installTray 应用启动后安装系统托盘（登录前即存在）。
func (u *uiApp) installTray() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	u.app.SetIcon(icons.Logo)
	u.trayMu.Lock()
	u.trayKind = -1 // 强制首次写入
	u.trayMu.Unlock()
	u.applyTray(trayKindIdle, true)
	desk.SetSystemTrayMenu(u.loginTrayMenu())
	// Fyne：SetSystemTrayMenu 可能用 App Icon 覆盖托盘，须在菜单之后再设状态图标
	u.forceTrayIcon(trayKindIdle)
}

// applyTray 按种类更新托盘图标；forceMenu 时重建菜单（登录/登出/连接成功时）。
func (u *uiApp) applyTray(kind trayKind, forceMenu bool) {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	u.trayMu.Lock()
	changed := u.trayKind != kind
	u.trayKind = kind
	needMenu := forceMenu || changed
	u.trayMu.Unlock()

	if needMenu {
		if u.getEngine() != nil {
			desk.SetSystemTrayMenu(u.mainTrayMenu())
		} else {
			desk.SetSystemTrayMenu(u.loginTrayMenu())
		}
	}
	// 菜单之后必须重设图标（Fyne/Windows 已知会回落 App Icon）
	if changed || needMenu {
		u.forceTrayIcon(kind)
	}
}

func (u *uiApp) forceTrayIcon(kind trayKind) {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	switch kind {
	case trayKindConnected:
		desk.SetSystemTrayIcon(icons.Connected)
	case trayKindConnecting:
		desk.SetSystemTrayIcon(icons.Connecting)
	case trayKindError:
		desk.SetSystemTrayIcon(icons.Error)
	default:
		desk.SetSystemTrayIcon(icons.Idle)
	}
}

// syncTrayFromEngine 根据 Engine 状态刷新托盘（登录中 eng 已有、主窗轮询共用）。
func (u *uiApp) syncTrayFromEngine(forceMenu bool) {
	kind := trayKindIdle
	menuKey := ""
	eng := u.getEngine()
	if eng == nil {
		u.applyTray(kind, forceMenu)
		return
	}
	st := eng.State()
	errMsg := eng.LastError()
	switch {
	case st == clientapp.StateConnected:
		kind = trayKindConnected
		// 含分流/托管数量，策略热更新后也能刷菜单
		menuKey = fmt.Sprintf("up:%s:a%d:m%d", eng.VPNIP(), len(eng.AllowedIPs()), len(eng.ManagedRoutes()))
	case st == clientapp.StateConnecting || st == clientapp.StateReconnecting:
		kind = trayKindConnecting
		menuKey = "connecting"
	case errMsg != "" && st == clientapp.StateIdle:
		kind = trayKindError
		menuKey = "err"
	default:
		kind = trayKindIdle
		menuKey = "idle"
	}
	u.trayMu.Lock()
	if menuKey != u.trayMenuKey {
		forceMenu = true
		u.trayMenuKey = menuKey
	}
	u.trayMu.Unlock()
	u.applyTray(kind, forceMenu)
}

// refreshTrayMenu 强制重建托盘菜单并恢复当前状态图标。
func (u *uiApp) refreshTrayMenu() {
	u.trayMu.Lock()
	k := u.trayKind
	if k < 0 {
		k = trayKindIdle
	}
	u.trayMu.Unlock()
	u.applyTray(k, true)
}

func (u *uiApp) loginTrayMenu() *fyne.Menu {
	return fyne.NewMenu(brand.Name,
		fyne.NewMenuItem("显示登录窗口", func() { u.showLoginWindow() }),
		u.configMenuItem(),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("退出", func() { u.quitApp() }),
	)
}

func (u *uiApp) mainTrayMenu() *fyne.Menu {
	items := []*fyne.MenuItem{}
	eng := u.getEngine()
	if eng != nil && eng.State() == clientapp.StateConnected {
		ip := eng.VPNIP()
		status := fyne.NewMenuItem("状态: 已连接 "+ip, nil)
		status.Disabled = true
		items = append(items, status)
		items = append(items, u.routesMenuItem(eng))
		items = append(items, fyne.NewMenuItemSeparator())
	}
	items = append(items,
		fyne.NewMenuItem("显示主窗口", func() { u.showMainWindow() }),
		fyne.NewMenuItem("重新连接", func() { u.reconnectVPN() }),
		u.configMenuItem(),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("退出登录", func() { u.doLogout() }),
		fyne.NewMenuItem("退出", func() { u.quitApp() }),
	)
	return fyne.NewMenu(brand.Name, items...)
}

// routesMenuItem 「本机路由」只读子菜单：本机TUN + 分流 AllowedIPs + 对端托管。
func (u *uiApp) routesMenuItem(eng *clientapp.Engine) *fyne.MenuItem {
	parent := fyne.NewMenuItem("本机路由", nil)
	allowed := eng.AllowedIPs()
	managed := eng.ManagedRoutes()
	logger.Debug("tray_routes allowed_n=%d managed_n=%d", len(allowed), len(managed))
	lines := trayRouteLines(eng.VPNSubnet(), eng.VPNIP(), eng.Gateway(), allowed, managed)
	children := make([]*fyne.MenuItem, 0, len(lines))
	for _, line := range lines {
		it := fyne.NewMenuItem(line, nil)
		it.Disabled = true
		children = append(children, it)
	}
	parent.ChildMenu = fyne.NewMenu("", children...)
	return parent
}

func (u *uiApp) showLoginWindow() {
	if u.loginWin == nil {
		u.showLogin("")
		return
	}
	u.loginWin.Show()
	u.loginWin.CenterOnScreen()
}

func (u *uiApp) showMainWindow() {
	if u.mainWin == nil {
		return
	}
	u.mainWin.Show()
	u.mainWin.CenterOnScreen()
}

func (u *uiApp) reconnectVPN() {
	if u.getEngine() == nil {
		return
	}
	if !u.beginEngineOp() {
		u.appendLog("正在处理网络，请稍候…")
		return
	}
	creds := clientapp.Credentials{
		Username: strings.TrimSpace(u.userEntry.Text),
		Password: u.passEntry.Text,
	}
	u.stopPoll()
	if u.statusLbl != nil {
		u.statusLbl.SetText("状态: 正在重新连接…")
	}
	u.applyTray(trayKindConnecting, true)
	u.appendLog("手动重新连接（清理后重拨，可能需数秒）…")
	old := u.takeEngine()
	u.stopEngineAsync(old, func() {
		eng := clientapp.NewEngine(u.cfg)
		eng.SetCredentials(creds)
		u.setEngine(eng)
		_ = eng.Start()
		u.startPoll()
		u.endEngineOp()
		u.applyTray(trayKindConnecting, true)
	})
}

func (u *uiApp) quitApp() {
	if !u.beginEngineOp() {
		u.appendLog("正在退出，请稍候…")
		return
	}
	u.stopPoll()
	if u.statusLbl != nil {
		u.statusLbl.SetText("状态: 正在退出（清理网络）…")
	}
	u.appendLog("正在退出（清理网络可能需数秒）…")
	logger.Info("gui_quit begin")
	eng := u.takeEngine()
	u.stopEngineAsync(eng, func() {
		logger.Info("gui_quit done")
		logger.SetSink(nil)
		_ = logger.Close()
		u.endEngineOp()
		u.app.Quit()
	})
}
