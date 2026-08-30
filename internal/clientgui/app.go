package clientgui

import (
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"haovpn/internal/clientapp"
	"haovpn/internal/config"
	"haovpn/internal/credentials"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
)

// uiApp 桌面客户端 UI 状态（登录窗、主窗、Engine 与日志缓冲）。
type uiApp struct {
	app        fyne.App
	configPath string
	cfg        *config.ClientConfig
	eng        *clientapp.Engine

	loginWin fyne.Window
	mainWin  fyne.Window

	statusLbl *widget.Label
	vpnIPLbl  *widget.Label
	logEntry  *widget.Entry
	logMu     sync.Mutex
	logLines  []string
	logSyncing bool // 程序 SetText 时忽略 OnChanged，防止用户编辑污染

	serverEntry  *widget.Entry
	userEntry    *widget.Entry
	passEntry    *widget.Entry
	localLansEntry *widget.Entry // 可选本地网段，逗号/换行分隔；空=关闭 via
	rememberPass *widget.Check
	errLbl       *widget.Label
	cfgPathLbl   *widget.Label

	pollStop chan struct{}

	// engOpBusy：登出/手动重连等正在后台 Stop，防连点卡死与竞态
	engOpMu   sync.Mutex
	engOpBusy bool

	// 托盘当前种类（SetSystemTrayMenu 会冲掉图标，须随后 forceTrayIcon）
	trayMu       sync.Mutex
	trayKind     trayKind
	trayMenuKey  string // 已连接时含 VPN IP，变化则重建菜单
}

// newUI 构造 UI 控制器并注册 logger sink，将日志行转发到 appendLog。
func newUI(a fyne.App, configPath string) *uiApp {
	u := &uiApp{app: a, configPath: configPath, logLines: make([]string, 0, logDisplayKeep)}
	logger.SetSink(func(_ logger.Level, line string) {
		u.appendLog(line)
	})
	return u
}

func (u *uiApp) loadOrCreateConfig() *config.ClientConfig {
	return config.LoadClientOrDefaults(u.configPath)
}

// showMain 打开主窗口（状态、日志、托盘），并在后台轮询 Engine 状态。
func (u *uiApp) showMain() {
	w := u.app.NewWindow("HaoVPN")
	u.mainWin = w
	w.Resize(fyne.NewSize(680, 520))

	u.statusLbl = widget.NewLabel("状态: 连接中…")
	u.vpnIPLbl = widget.NewLabel("VPN IP: —")
	u.logEntry = widget.NewMultiLineEntry()
	// 保持启用以便深色字可读；用 OnChanged 丢弃用户编辑（仅程序 SetText）
	u.logEntry.SetMinRowsVisible(18)
	u.logEntry.OnChanged = func(s string) {
		if u.logSyncing {
			return
		}
		u.logMu.Lock()
		want := strings.Join(u.logLines, "\n")
		u.logMu.Unlock()
		if s != want {
			u.logSyncing = true
			u.logEntry.SetText(want)
			u.logSyncing = false
		}
	}

	reconnectBtn := widget.NewButton("重新连接", func() { u.reconnectVPN() })
	logoutBtn := widget.NewButton("退出登录", func() { u.doLogout() })
	saveSvcBtn := widget.NewButton("保存供服务使用", func() {
		user := strings.TrimSpace(u.userEntry.Text)
		pass := u.passEntry.Text
		if user == "" || pass == "" {
			u.appendLog("保存服务凭据失败: 请先填写账号密码")
			return
		}
		if err := credentials.SaveService(user, pass); err != nil {
			u.appendLog("保存服务凭据失败: " + err.Error())
			return
		}
		u.appendLog("已保存 Windows 服务凭据")
	})
	quitBtn := widget.NewButton("退出程序", func() { u.quitApp() })

	top := container.NewVBox(u.statusLbl, u.vpnIPLbl, widget.NewSeparator())
	btns := container.NewHBox(reconnectBtn, logoutBtn, saveSvcBtn, layout.NewSpacer(), quitBtn)
	w.SetContent(container.NewBorder(top, btns, nil, nil, container.NewPadded(u.logEntry)))
	// 登录阶段 sink 已写入 logLines，须刷入新建的日志框，否则首屏空白
	u.flushLogView()
	w.SetCloseIntercept(func() { w.Hide() })
	w.CenterOnScreen()

	u.refreshTrayMenu()

	// 无窗口模式：创建主窗但不弹出，仅托盘；用户可「显示主窗口」
	if u.cfg != nil && u.cfg.GUI.StartMinimized {
		w.Hide()
	} else {
		w.Show()
	}
	u.startPoll()
}

// doLogout 停止 VPN、关闭主窗并回到登录窗。
//
// Stop（含 ICS 清理）在后台执行，避免 UI 线程卡死；完成后切回登录窗。
func (u *uiApp) doLogout() {
	if !u.beginEngineOp() {
		u.appendLog("正在断开，请稍候…")
		return
	}
	u.stopPoll()
	if u.statusLbl != nil {
		u.statusLbl.SetText("状态: 正在断开…")
	}
	u.appendLog("正在退出登录（清理网络可能需数秒）…")
	eng := u.takeEngine()
	u.stopEngineAsync(eng, func() {
		u.finishLogoutUI()
		u.endEngineOp()
	})
}

// finishLogoutUI 登出后台 Stop 完成后的界面切换（须在 UI 线程）。
func (u *uiApp) finishLogoutUI() {
	if u.mainWin != nil {
		u.mainWin.Hide()
		u.mainWin.Close()
		u.mainWin = nil
	}
	if u.passEntry != nil && !u.cfg.Auth.RememberPassword {
		u.passEntry.SetText("")
	}
	if u.loginWin != nil {
		u.loginWin.Show()
		u.loginWin.CenterOnScreen()
	} else {
		u.showLogin("")
	}
	u.trayMu.Lock()
	u.trayMenuKey = ""
	u.trayMu.Unlock()
	u.applyTray(trayKindIdle, true)
}

// startPoll 后台定时刷新连接状态、VPN IP 与托盘图标（500ms）。
func (u *uiApp) startPoll() {
	u.stopPoll()
	u.pollStop = make(chan struct{})
	stop := u.pollStop
	safeutil.GoSafe("gui-status-poll", func() {
		safeutil.RunTickerStop(stop, 500*time.Millisecond, func() {
			eng := u.getEngine()
			if eng == nil {
				return
			}
			st := eng.State()
			ip := eng.VPNIP()
			errMsg := eng.LastError()
			ksOK := eng.KillSwitchOK()
			fyne.Do(func() {
				if u.statusLbl != nil {
					txt := "状态: " + st.String()
					if !ksOK && errMsg != "" {
						txt += " | " + errMsg
					} else if errMsg != "" && st != clientapp.StateConnected {
						txt += " | " + errMsg
					}
					u.statusLbl.SetText(txt)
					if ip != "" {
						u.vpnIPLbl.SetText("VPN IP: " + ip)
					} else {
						u.vpnIPLbl.SetText("VPN IP: —")
					}
				}
				// 仅状态变化时更新图标；菜单仅在已连接且需展示 IP/路由时按需 force
				u.syncTrayFromEngine(false)
			})
		})
	})
}

// stopPoll 停止状态轮询 goroutine。
func (u *uiApp) stopPoll() {
	if u.pollStop != nil {
		close(u.pollStop)
		u.pollStop = nil
	}
}

// shutdown 在 a.Run 返回后做兜底清理（正常退出已由 quitApp 异步 Stop）。
//
// 若仍残留 Engine（异常关窗等），后台 Stop 并最多等待 15s，超时 Warn 后仍退出以免永久挂死。
func (u *uiApp) shutdown() {
	u.stopPoll()
	logger.SetSink(nil)
	if eng := u.takeEngine(); eng != nil {
		done := make(chan struct{})
		safeutil.GoSafe("gui-shutdown-stop", func() {
			logger.Info("gui_shutdown_stop begin")
			eng.Stop()
			logger.Info("gui_shutdown_stop done")
			close(done)
		})
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			logger.Warn("gui_shutdown_stop 超时 15s，继续退出（可能残留 ICS，可手动检查）")
		}
	}
	_ = logger.Close()
}
