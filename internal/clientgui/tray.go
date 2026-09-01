package clientgui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/systray"

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
	trayKindDisconnecting // 退出登录/退出程序 Stop 中
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
	u.refreshTrayTooltip()
}

// applyTray 按种类更新托盘图标；forceMenu 时重建菜单（登录/登出/连接成功时）。
//
// 即使非 desktop.App（单测/无托盘环境）也更新 trayKind 并刷 tip 输入态，
// 保证状态机与 sticky 逻辑可验证、与有托盘时一致。
func (u *uiApp) applyTray(kind trayKind, forceMenu bool) {
	u.trayMu.Lock()
	changed := u.trayKind != kind
	u.trayKind = kind
	needMenu := forceMenu || changed
	u.trayMu.Unlock()

	desk, ok := u.app.(desktop.App)
	if !ok {
		u.refreshTrayTooltip()
		return
	}

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
	u.refreshTrayTooltip()
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
	case trayKindDisconnecting:
		desk.SetSystemTrayIcon(icons.Connecting) // 视觉上表示忙碌；文案为「正在断开」
	default:
		desk.SetSystemTrayIcon(icons.Idle)
	}
}

// syncTrayFromEngine 根据 Engine 状态刷新托盘（登录中 eng 已有、主窗轮询共用）。
func (u *uiApp) syncTrayFromEngine(forceMenu bool) {
	pres := trayPresentationFromEngine(u.getEngine())
	u.trayMu.Lock()
	if pres.MenuKey != u.trayMenuKey {
		forceMenu = true
		u.trayMenuKey = pres.MenuKey
	}
	u.trayMu.Unlock()
	u.applyTray(pres.Kind, forceMenu)
}

// refreshTrayTooltip 按 Engine/配置刷新悬停气泡（Fyne 无官方 API，经 fyne.io/systray）。
// engOpBusy（正在 Stop）时强制「正在断开…」，避免残留「正在连接」。
func (u *uiApp) refreshTrayTooltip() {
	tip := formatTrayTooltip(u.trayTooltipInputNow())
	u.trayMu.Lock()
	if tip == u.lastTooltip {
		u.trayMu.Unlock()
		return
	}
	u.lastTooltip = tip
	u.trayMu.Unlock()
	systray.SetTooltip(tip)
}

// trayTooltipInputNow 组装当前 tip 输入（纯逻辑，便于单测；不碰 systray）。
//
// 优先级：
//  1. engOpBusy 且无 sticky →「正在断开」（登出/退出/手动重连清理）；
//  2. engOpBusy 且有 sticky → 仍展示错误（登录失败清理中，勿盖掉原因）；
//  3. 有 Engine → State/LastError；
//  4. 否则 trayStickyErr（eng 已清、Stop 可能仍在跑）。
func (u *uiApp) trayTooltipInputNow() trayTooltipInput {
	in := trayTooltipInput{State: clientapp.StateIdle}
	u.trayMu.Lock()
	sticky := u.trayStickyErr
	u.trayMu.Unlock()
	if u.isEngineOpBusy() {
		if sticky != "" {
			in.LastError = sticky
			return in
		}
		in.Phase = trayTipDisconnecting
		return in
	}
	if u.cfg != nil {
		in.Server = strings.TrimSpace(u.cfg.Server.Address)
	}
	eng := u.getEngine()
	if eng != nil {
		in.State = eng.State()
		in.VPNIP = eng.VPNIP()
		in.Since = eng.ConnectedSince()
		in.LastError = eng.LastError()
		return in
	}
	in.LastError = sticky
	return in
}

// setTrayStickyErr 在无 Engine 时仍让托盘 tip 显示登录失败原因。
func (u *uiApp) setTrayStickyErr(msg string) {
	u.trayMu.Lock()
	u.trayStickyErr = strings.TrimSpace(msg)
	u.trayMu.Unlock()
}

// clearTrayStickyErr 开始新连接或主动退出登录时清掉残留失败 tip。
func (u *uiApp) clearTrayStickyErr() {
	u.trayMu.Lock()
	u.trayStickyErr = ""
	u.trayMu.Unlock()
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
	creds := clientapp.Credentials{
		Username: strings.TrimSpace(u.userEntry.Text),
		Password: u.passEntry.Text,
	}
	if !u.beginEngineOp() {
		// busy：排队重连并 bump opGen，打断进行中 HardRestart 的 DNS/Start
		if u.setPendingIntent(intentReconnect, creds) {
			u.appendLog("将中断当前网络操作并重新连接…")
			logger.Info("gui_reconnect deferred")
			return
		}
		u.appendLog("正在处理网络，请稍候…")
		return
	}
	old := u.beginNetworkOp(networkOpReconnect)
	// HardRestart 内含 Stop；勿再 stopEngineAsync，避免双重 Stop / onDone 竞态。
	u.reconnectVPNAfterStop(old, creds)
}

func (u *uiApp) quitApp() {
	if !u.beginEngineOp() {
		if u.setPendingIntent(intentQuit, clientapp.Credentials{}) {
			u.appendLog("将在当前清理完成后退出…")
			logger.Info("gui_quit deferred")
			return
		}
		u.appendLog("正在退出，请稍候…")
		return
	}
	eng := u.beginNetworkOp(networkOpQuit)
	u.stopEngineAsync(eng, func() {
		if u.applyPendingAfterEngineOp() {
			return
		}
		u.finishQuitApp()
	})
}
