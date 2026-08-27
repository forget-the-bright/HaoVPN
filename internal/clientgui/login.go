package clientgui

import (
	"net"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"haovpn/internal/brand"
	"haovpn/internal/clientapp"
	"haovpn/internal/platform"
	"haovpn/internal/version"
)

// showLogin 展示登录窗；elevHint 非空时在错误区提示须管理员或提权失败原因。
func (u *uiApp) showLogin(elevHint string) {
	u.cfg = u.loadOrCreateConfig()
	if err := u.cfg.Log.InitGlobal(); err != nil {
		a := u.app
		w := a.NewWindow(brand.Name)
		dialog.ShowError(err, w)
		w.Show()
		a.Run()
		return
	}

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

// tryConnect 校验表单、启动 Engine 并切换到主窗口。
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
	u.cfg.ApplyDefaults()
	if err := u.cfg.Validate(); err != nil {
		u.errLbl.SetText(err.Error())
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
