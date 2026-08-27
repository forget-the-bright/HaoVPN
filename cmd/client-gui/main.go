// cmd/client-gui 是 HaoVPN 跨平台桌面客户端（Fyne：登录 / 日志 / 托盘）。
package main

import (
	"flag"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"haovpn/internal/brand"
	"haovpn/internal/clientapp"
	"haovpn/internal/config"
	"haovpn/internal/credentials"
	"haovpn/internal/logger"
	"haovpn/internal/platform"
	"haovpn/internal/safeutil"
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
		configPath = resolveClientConfigPath()
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

	a := app.NewWithID(brand.GUIAppID)
	a.Settings().SetTheme(readableTheme{})
	ui := newUI(a, configPath)
	ui.showLogin(elevHint)
	a.Run()
	ui.shutdown()
}

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

	serverEntry *widget.Entry
	userEntry   *widget.Entry
	passEntry   *widget.Entry
	killSwitch  *widget.Check
	errLbl      *widget.Label
	cfgPathLbl  *widget.Label

	pollStop chan struct{}
}

func newUI(a fyne.App, configPath string) *uiApp {
	u := &uiApp{app: a, configPath: configPath, logLines: make([]string, 0, 200)}
	logger.SetSink(func(_ logger.Level, line string) {
		u.appendLog(line)
	})
	return u
}

func (u *uiApp) loadOrCreateConfig() *config.ClientConfig {
	cfg, _, err := config.LoadClient(u.configPath)
	if err != nil || cfg == nil {
		cfg = &config.ClientConfig{}
		cfg.Server.Address = "REPLACE_WITH_SERVER_IP:8443"
		cfg.Tun.Name = brand.DefaultTunName
		cfg.Tun.MTU = 1420
		cfg.Server.HeartbeatIntervalSec = 15
		cfg.Server.HeartbeatTimeoutSec = 90
		cfg.Server.DialTimeoutSec = 3
		cfg.Reconnect.InitialSec = 1
		cfg.Reconnect.MaxSec = 3
		cfg.Log.Level = "info"
		cfg.Log.File = "./logs/client.log"
		cfg.Server.TLS.InsecureSkipVerify = false
	}
	return cfg
}

func (u *uiApp) showLogin(elevHint string) {
	u.cfg = u.loadOrCreateConfig()
	_ = logger.Init(logger.Config{
		Level: u.cfg.Log.Level, File: u.cfg.Log.File,
		MaxSizeMB: u.cfg.Log.MaxSizeMB, MaxBackups: u.cfg.Log.MaxBackups,
	})

	w := u.app.NewWindow("HaoVPN 登录")
	u.loginWin = w
	w.Resize(fyne.NewSize(460, 420))

	u.serverEntry = widget.NewEntry()
	u.serverEntry.SetText(u.cfg.Server.Address)
	u.serverEntry.SetPlaceHolder("host:8443")
	u.userEntry = widget.NewEntry()
	user, _ := u.cfg.ResolveAuth()
	u.userEntry.SetText(user)
	u.userEntry.SetPlaceHolder("账号")
	u.passEntry = widget.NewPasswordEntry()
	u.passEntry.SetPlaceHolder("密码")
	u.killSwitch = widget.NewCheck("断线阻断工控网段（杀开关）", nil)
	u.killSwitch.SetChecked(u.cfg.Security.KillSwitch)
	u.errLbl = widget.NewLabel("")
	u.errLbl.Wrapping = fyne.TextWrapWord
	if elevHint != "" {
		u.errLbl.SetText(elevHint)
	}
	u.cfgPathLbl = widget.NewLabel("配置: " + u.configPath)
	u.cfgPathLbl.Wrapping = fyne.TextWrapWord

	title := widget.NewLabelWithStyle("HaoVPN", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	sub := widget.NewLabelWithStyle(version.String(), fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	connectBtn := widget.NewButton("连接", func() { u.tryConnect() })
	connectBtn.Importance = widget.HighImportance

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("服务器"), u.serverEntry,
		widget.NewLabel("账号"), u.userEntry,
		widget.NewLabel("密码"), u.passEntry,
		widget.NewLabel(""), u.killSwitch,
	)
	content := container.NewVBox(
		layout.NewSpacer(),
		title, sub,
		widget.NewSeparator(),
		form,
		u.cfgPathLbl,
		u.errLbl,
		connectBtn,
		layout.NewSpacer(),
	)
	w.SetContent(container.NewPadded(content))
	w.Show()
}

func (u *uiApp) tryConnect() {
	if !platform.IsAdmin() {
		u.errLbl.SetText("须以管理员运行（TUN/路由/杀开关）")
		return
	}
	addr := strings.TrimSpace(u.serverEntry.Text)
	user := strings.TrimSpace(u.userEntry.Text)
	pass := u.passEntry.Text
	if addr == "" || user == "" || pass == "" {
		u.errLbl.SetText("请填写服务器、账号和密码")
		return
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		u.errLbl.SetText("服务器地址须为 host:port")
		return
	}
	u.cfg.Server.Address = addr
	u.cfg.Auth.Username = user
	u.cfg.Security.KillSwitch = u.killSwitch.Checked
	if err := ensureMinimalValid(u.cfg); err != nil {
		u.errLbl.SetText(err.Error())
		return
	}
	if !u.cfg.Server.TLS.InsecureSkipVerify && strings.TrimSpace(u.cfg.Server.TLS.CAFile) == "" {
		u.errLbl.SetText("请配置 server.tls.ca_file（server.crt）或启用跳过证书校验")
		return
	}

	if u.eng != nil {
		u.eng.Stop()
	}
	u.eng = clientapp.NewEngine(u.cfg)
	u.eng.SetCredentials(clientapp.Credentials{Username: user, Password: pass})
	if err := u.eng.Start(); err != nil {
		u.errLbl.SetText(err.Error())
		return
	}
	u.errLbl.SetText("")
	u.loginWin.Hide()
	u.showMain()
}

func ensureMinimalValid(cfg *config.ClientConfig) error {
	if cfg.Tun.Name == "" {
		cfg.Tun.Name = brand.DefaultTunName
	}
	if cfg.Tun.MTU <= 0 {
		cfg.Tun.MTU = 1420
	}
	if cfg.Server.HeartbeatIntervalSec <= 0 {
		cfg.Server.HeartbeatIntervalSec = 15
	}
	if cfg.Server.HeartbeatTimeoutSec <= 0 {
		cfg.Server.HeartbeatTimeoutSec = 90
	}
	if cfg.Server.DialTimeoutSec <= 0 {
		cfg.Server.DialTimeoutSec = 3
	}
	if cfg.Reconnect.InitialSec <= 0 {
		cfg.Reconnect.InitialSec = 1
	}
	if cfg.Reconnect.MaxSec <= 0 {
		cfg.Reconnect.MaxSec = 3
	}
	if cfg.Log.File == "" {
		cfg.Log.File = "./logs/client.log"
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	return cfg.Validate()
}

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

	reconnectBtn := widget.NewButton("重新连接", func() {
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
	})
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
	quitBtn := widget.NewButton("退出程序", func() {
		u.shutdown()
		u.app.Quit()
	})

	top := container.NewVBox(u.statusLbl, u.vpnIPLbl, widget.NewSeparator())
	btns := container.NewHBox(reconnectBtn, logoutBtn, saveSvcBtn, layout.NewSpacer(), quitBtn)
	w.SetContent(container.NewBorder(top, btns, nil, nil, container.NewPadded(u.logEntry)))
	w.SetCloseIntercept(func() { w.Hide() })

	if desk, ok := u.app.(desktop.App); ok {
		desk.SetSystemTrayIcon(theme.ComputerIcon())
		m := fyne.NewMenu("HaoVPN",
			fyne.NewMenuItem("显示主窗口", func() { w.Show() }),
			fyne.NewMenuItem("重新连接", func() { reconnectBtn.OnTapped() }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("退出登录", func() { u.doLogout() }),
			fyne.NewMenuItem("退出", func() {
				u.shutdown()
				u.app.Quit()
			}),
		)
		desk.SetSystemTrayMenu(m)
	}

	w.Show()
	u.startPoll()
}

func (u *uiApp) doLogout() {
	u.stopPoll()
	if u.eng != nil {
		u.eng.Stop()
		u.eng = nil
	}
	if u.mainWin != nil {
		u.mainWin.Hide()
		u.mainWin.Close()
		u.mainWin = nil
	}
	if u.passEntry != nil {
		u.passEntry.SetText("")
	}
	if u.loginWin != nil {
		u.loginWin.Show()
	} else {
		u.showLogin("")
	}
}

func (u *uiApp) startPoll() {
	u.stopPoll()
	u.pollStop = make(chan struct{})
	stop := u.pollStop
	safeutil.GoSafe("gui-status-poll", func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if u.eng == nil || u.statusLbl == nil {
					continue
				}
				st := u.eng.State()
				ip := u.eng.VPNIP()
				errMsg := u.eng.LastError()
				ksOK := u.eng.KillSwitchOK()
				fyne.Do(func() {
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
				})
			}
		}
	})
}

func (u *uiApp) stopPoll() {
	if u.pollStop != nil {
		close(u.pollStop)
		u.pollStop = nil
	}
}

func (u *uiApp) appendLog(line string) {
	u.logMu.Lock()
	u.logLines = append(u.logLines, line)
	if len(u.logLines) > 500 {
		u.logLines = u.logLines[len(u.logLines)-400:]
	}
	text := strings.Join(u.logLines, "\n")
	n := len(u.logLines)
	u.logMu.Unlock()
	if u.logEntry == nil {
		return
	}
	fyne.Do(func() {
		u.logSyncing = true
		u.logEntry.SetText(text)
		u.logEntry.CursorRow = n
		u.logSyncing = false
	})
}

func (u *uiApp) shutdown() {
	u.stopPoll()
	logger.SetSink(nil)
	if u.eng != nil {
		u.eng.Stop()
		u.eng = nil
	}
	_ = logger.Close()
}
