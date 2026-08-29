package clientgui

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"haovpn/internal/clientapp"
	"haovpn/internal/clientgui/icons"
	"haovpn/internal/config"
	"haovpn/internal/platform"
	"haovpn/internal/safeutil"
	"haovpn/internal/version"
)

// showLogin 展示登录窗；elevHint 非空时在错误区提示须管理员或提权失败原因。
func (u *uiApp) showLogin(elevHint string) {
	u.cfg = u.loadOrCreateConfig()
	if err := u.cfg.Log.InitGlobal(); err != nil {
		showFatalErrorOnApp(u.app, err)
		return
	}

	w := u.app.NewWindow("HaoVPN 登录")
	u.loginWin = w
	w.Resize(fyne.NewSize(480, 520))

	u.serverEntry = widget.NewEntry()
	u.serverEntry.SetText(u.cfg.Server.Address)
	u.serverEntry.SetPlaceHolder("host:8443")
	u.userEntry = widget.NewEntry()
	user, _ := u.cfg.ResolveAuth()
	u.userEntry.SetText(user)
	u.userEntry.SetPlaceHolder("账号")
	u.passEntry = widget.NewPasswordEntry()
	u.passEntry.SetPlaceHolder("密码")
	if u.cfg.Auth.RememberPassword && u.cfg.Auth.Password != "" {
		u.passEntry.SetText(u.cfg.Auth.Password)
	}
	u.localLansEntry = widget.NewMultiLineEntry()
	u.localLansEntry.SetPlaceHolder("可选：本机局域网段，如 192.168.31.0/24（空=关闭 via 出口）")
	u.localLansEntry.SetMinRowsVisible(2)
	if len(u.cfg.LocalLANs) > 0 {
		u.localLansEntry.SetText(strings.Join(u.cfg.LocalLANs, "\n"))
	}
	u.rememberPass = widget.NewCheck("记住密码", nil)
	u.rememberPass.SetChecked(u.cfg.Auth.RememberPassword)
	u.errLbl = widget.NewLabel("")
	u.errLbl.Wrapping = fyne.TextWrapWord
	if elevHint != "" {
		u.errLbl.SetText(elevHint)
	}
	u.cfgPathLbl = widget.NewLabel("配置: " + u.configPath)
	u.cfgPathLbl.Wrapping = fyne.TextWrapWord

	title := widget.NewLabelWithStyle("HaoVPN", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	sub := widget.NewLabelWithStyle(version.String(), fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	logo := icons.LogoImage()

	connectBtn := widget.NewButton("连接", func() { u.tryConnect() })
	connectBtn.Importance = widget.HighImportance

	form := container.New(layout.NewFormLayout(),
		widget.NewLabel("服务器"), u.serverEntry,
		widget.NewLabel("账号"), u.userEntry,
		widget.NewLabel("密码"), u.passEntry,
		widget.NewLabel("本地网段"), u.localLansEntry,
		widget.NewLabel(""), u.rememberPass,
	)
	content := container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(logo),
		title, sub,
		widget.NewSeparator(),
		form,
		u.cfgPathLbl,
		u.errLbl,
		connectBtn,
		layout.NewSpacer(),
	)
	w.SetContent(container.NewPadded(content))
	w.CenterOnScreen()
	w.SetCloseIntercept(func() { w.Hide() })
	w.Show()
}

// tryConnect 校验表单、启动 Engine；鉴权成功（WaitConnected）后进主界面，TUN 在后台继续配置。
//
// 密码错误、账号已在线等失败留在登录页并显示 errLbl。
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
	u.cfg.Server.Address = addr
	u.cfg.Auth.Username = user
	u.cfg.Auth.RememberPassword = u.rememberPass.Checked
	if u.rememberPass.Checked {
		u.cfg.Auth.Password = pass
	} else {
		u.cfg.Auth.Password = ""
	}
	u.cfg.LocalLANs = parseLocalLANsText(u.localLansEntry.Text)
	u.cfg.ApplyDefaults()
	if err := u.cfg.Validate(); err != nil {
		u.errLbl.SetText(err.Error())
		return
	}

	if u.getEngine() != nil {
		if !u.beginEngineOp() {
			u.errLbl.SetText("正在清理上一连接，请稍候…")
			return
		}
		old := u.takeEngine()
		u.stopPoll()
		u.errLbl.SetText("正在清理上一连接…")
		u.stopEngineAsync(old, func() {
			u.endEngineOp()
			u.tryConnect() // 清理完成后再发起本次连接
		})
		return
	}
	eng := clientapp.NewEngine(u.cfg)
	eng.SetFailFast(true) // 登录页：首次失败立即提示，勿空转重连
	eng.SetCredentials(clientapp.Credentials{Username: user, Password: pass})
	// 鉴权成功进主窗后若 TUN/路由失败，回登录并红字（勿停在「假连接」主界面）
	eng.SetOnDataplaneFailed(func(msg string) {
		fyne.Do(func() {
			if u.getEngine() == nil {
				return
			}
			u.doLogout()
			if u.errLbl != nil {
				u.errLbl.SetText(msg)
			}
		})
	})
	u.setEngine(eng)
	u.errLbl.SetText("正在连接…")
	u.applyTray(trayKindConnecting, true)
	if err := eng.Start(); err != nil {
		u.errLbl.SetText(err.Error())
		u.applyTray(trayKindError, true)
		return
	}

	// 登录等待期间也轮询托盘状态（主窗尚未创建）
	u.startPoll()
	safeutil.GoSafe("gui-wait-connected", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		err := eng.WaitConnected(ctx)
		if err != nil {
			// Stop 可能含 ICS 清理，勿在 fyne.Do 里同步执行
			eng.Stop()
			fyne.Do(func() {
				u.clearEngineIf(eng)
				u.stopPoll()
				msg := err.Error()
				if le := eng.LastError(); le != "" {
					msg = le
				}
				if ctx.Err() != nil && msg == context.DeadlineExceeded.Error() {
					msg = "连接超时，请检查服务器地址、网络与密码"
				}
				u.errLbl.SetText(msg)
				u.applyTray(trayKindError, true)
			})
			return
		}
		fyne.Do(func() {
			if !u.isSameEngine(eng) {
				return // 用户已重新点连接或退出
			}
			if err := config.SaveClient(u.configPath, u.cfg); err != nil {
				u.errLbl.SetText("连接成功但保存配置失败: " + err.Error())
				return
			}
			u.errLbl.SetText("")
			u.loginWin.Hide()
			u.applyTray(trayKindConnected, true)
			u.showMain()
		})
	})
}

// parseLocalLANsText 解析 GUI 多行/逗号分隔的本地网段文本。
func parseLocalLANsText(s string) []string {
	s = strings.ReplaceAll(s, ",", "\n")
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
