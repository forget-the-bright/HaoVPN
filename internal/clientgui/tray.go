package clientgui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"haovpn/internal/brand"
	"haovpn/internal/clientapp"
	"haovpn/internal/clientgui/icons"
	"haovpn/internal/tunnel"
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
		if u.eng != nil {
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
	if u.eng == nil {
		u.applyTray(kind, forceMenu)
		return
	}
	st := u.eng.State()
	errMsg := u.eng.LastError()
	switch {
	case st == clientapp.StateConnected:
		kind = trayKindConnected
		menuKey = "up:" + u.eng.VPNIP()
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
		fyne.NewMenuItem("退出", func() { u.quitApp() }),
	)
}

func (u *uiApp) mainTrayMenu() *fyne.Menu {
	items := []*fyne.MenuItem{}
	if u.eng != nil && u.eng.State() == clientapp.StateConnected {
		ip := u.eng.VPNIP()
		status := fyne.NewMenuItem("状态: 已连接 "+ip, nil)
		status.Disabled = true
		items = append(items, status)
		items = append(items, u.managedRoutesMenuItem())
		items = append(items, fyne.NewMenuItemSeparator())
	}
	items = append(items,
		fyne.NewMenuItem("显示主窗口", func() { u.showMainWindow() }),
		fyne.NewMenuItem("重新连接", func() { u.reconnectVPN() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("退出登录", func() { u.doLogout() }),
		fyne.NewMenuItem("退出", func() { u.quitApp() }),
	)
	return fyne.NewMenu(brand.Name, items...)
}

// managedRoutesMenuItem 托管路由只读子菜单（对齐 ZeroTier Managed Routes）。
func (u *uiApp) managedRoutesMenuItem() *fyne.MenuItem {
	parent := fyne.NewMenuItem("托管路由", nil)
	var children []*fyne.MenuItem
	if u.eng != nil {
		gw := strings.TrimSpace(u.eng.Gateway())
		vpnIP := strings.TrimSpace(u.eng.VPNIP())
		localLine := "VPN 本机"
		if vpnIP != "" && gw != "" {
			localLine = fmt.Sprintf("%s via %s (本机TUN)", deriveVPNSubnetHint(vpnIP), gw)
		} else if gw != "" {
			localLine = fmt.Sprintf("网关 %s via 本机TUN", gw)
		}
		loc := fyne.NewMenuItem(localLine, nil)
		loc.Disabled = true
		children = append(children, loc)

		for _, mr := range u.eng.ManagedRoutes() {
			line := formatManagedRouteLine(mr)
			it := fyne.NewMenuItem(line, nil)
			it.Disabled = true
			children = append(children, it)
		}
	}
	if len(children) == 1 {
		hint := fyne.NewMenuItem("（无对端托管路由）", nil)
		hint.Disabled = true
		children = append(children, hint)
	}
	parent.ChildMenu = fyne.NewMenu("", children...)
	return parent
}

func formatManagedRouteLine(mr tunnel.ManagedRoute) string {
	dest := strings.TrimSpace(mr.Dest)
	via := strings.TrimSpace(mr.ViaIP)
	if via != "" {
		return fmt.Sprintf("%s via %s", dest, via)
	}
	name := strings.TrimSpace(mr.ViaUsername)
	if name == "" {
		name = "via"
	}
	return fmt.Sprintf("%s via %s(离线)", dest, name)
}

func deriveVPNSubnetHint(vpnIP string) string {
	parts := strings.Split(vpnIP, ".")
	if len(parts) == 4 {
		return parts[0] + "." + parts[1] + "." + parts[2] + ".0/24"
	}
	return vpnIP
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
	u.applyTray(trayKindConnecting, true)
	_ = u.eng.Start()
	u.appendLog("手动重新连接…")
}

func (u *uiApp) quitApp() {
	u.shutdown()
	u.app.Quit()
}
