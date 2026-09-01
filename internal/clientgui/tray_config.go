package clientgui

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"

	"haovpn/internal/clientapp"
	"haovpn/internal/config"
	"haovpn/internal/logger"
)

// configMenuItem 托盘「配置」子菜单。
func (u *uiApp) configMenuItem() *fyne.MenuItem {
	parent := fyne.NewMenuItem("配置", nil)
	u.ensureConfigLoaded()

	autoItem := fyne.NewMenuItem("自动连接", func() { u.toggleAutoConnect() })
	autoItem.Checked = u.cfg.GUI.AutoConnect

	minItem := fyne.NewMenuItem("无窗口模式（仅托盘）", func() { u.toggleStartMinimized() })
	minItem.Checked = u.cfg.GUI.StartMinimized

	logonOn, _, _ := clientapp.LogonAutostartStatus()
	logonItem := fyne.NewMenuItem("开机自启（登录后起本程序）", func() { u.toggleLogonAutostart() })
	logonItem.Checked = logonOn

	inst, _, _, _ := clientapp.ServiceAutostartStatus()
	svcItem := fyne.NewMenuItem("开机自启（服务，无托盘）", func() { u.toggleServiceAutostart() })
	svcItem.Checked = inst

	ksItem := fyne.NewMenuItem("杀开关（下次连接生效）", func() { u.toggleKillSwitch() })
	ksItem.Checked = u.cfg.Security.KillSwitch

	pathHint := fyne.NewMenuItem("配置: "+u.configPath, nil)
	pathHint.Disabled = true

	parent.ChildMenu = fyne.NewMenu("", autoItem, minItem, logonItem, svcItem, ksItem, pathHint)
	return parent
}

func (u *uiApp) saveGUIConfig(reason string) {
	if err := config.SaveClient(u.configPath, u.cfg); err != nil {
		logger.Warn("保存配置失败 (%s): %v", reason, err)
		u.appendLog("保存配置失败: " + err.Error())
		return
	}
	logger.Info("gui_cfg saved reason=%s auto_connect=%v start_minimized=%v kill_switch=%v",
		reason, u.cfg.GUI.AutoConnect, u.cfg.GUI.StartMinimized, u.cfg.Security.KillSwitch)
	u.refreshTrayMenu()
}

func (u *uiApp) toggleAutoConnect() {
	if u.cfg == nil {
		return
	}
	if !u.cfg.GUI.AutoConnect {
		u.cfg.GUI.AutoConnect = true
		if !u.cfg.CanAutoConnect() {
			u.cfg.GUI.AutoConnect = false
			u.appendLog("开启自动连接失败：请先勾选「记住密码」并成功连接一次以保存密码")
			u.refreshTrayMenu()
			return
		}
	} else {
		u.cfg.GUI.AutoConnect = false
	}
	u.saveGUIConfig("auto_connect")
}

func (u *uiApp) toggleStartMinimized() {
	if u.cfg == nil {
		return
	}
	u.cfg.GUI.StartMinimized = !u.cfg.GUI.StartMinimized
	u.saveGUIConfig("start_minimized")
	u.appendLog("无窗口模式已更新（下次启动生效；当前可继续用托盘显示窗口）")
}

func (u *uiApp) toggleKillSwitch() {
	if u.cfg == nil {
		return
	}
	u.cfg.Security.KillSwitch = !u.cfg.Security.KillSwitch
	u.saveGUIConfig("kill_switch")
	u.appendLog("杀开关已写入配置，下次连接生效")
}

func (u *uiApp) toggleLogonAutostart() {
	if !u.requireAdmin("开机自启（登录后）须以管理员运行", u.appendLog) {
		return
	}
	on, _, _ := clientapp.LogonAutostartStatus()
	if on {
		if err := clientapp.LogonAutostartDisable(); err != nil {
			u.appendLog("取消登录自启失败: " + err.Error())
			return
		}
		u.appendLog("已取消登录后开机自启")
		u.refreshTrayMenu()
		return
	}
	exe, err := clientapp.ResolveClientExecutable()
	if err != nil {
		u.appendLog("解析程序路径失败: " + err.Error())
		return
	}
	cfgPath, err := absPath(u.configPath)
	if err != nil {
		cfgPath = u.configPath
	}
	if err := clientapp.LogonAutostartEnable(exe, cfgPath); err != nil {
		u.appendLog("启用登录自启失败: " + err.Error())
		return
	}
	u.appendLog("已启用登录后开机自启（计划任务，最高权限）")
	u.refreshTrayMenu()
}

func (u *uiApp) toggleServiceAutostart() {
	if !u.requireAdmin("服务开机自启须以管理员运行", u.appendLog) {
		return
	}
	inst, _, _, _ := clientapp.ServiceAutostartStatus()
	if inst {
		if err := clientapp.ServiceAutostartDisable(); err != nil {
			u.appendLog("卸载服务失败: " + err.Error())
			return
		}
		u.appendLog("已关闭服务开机自启（已卸载）")
		u.refreshTrayMenu()
		return
	}
	user, pass := u.cfg.ResolveAuth()
	if user == "" || pass == "" {
		u.appendLog("启用服务前请先记住密码（服务凭据需要账号密码）")
		return
	}
	if err := clientapp.SaveServiceCredentials(user, pass); err != nil {
		u.appendLog("保存服务凭据失败: " + err.Error())
		return
	}
	exe, err := clientapp.ResolveClientExecutable()
	if err != nil {
		u.appendLog("解析程序路径失败: " + err.Error())
		return
	}
	startNow := u.getEngine() == nil
	if !startNow {
		u.appendLog("当前界面已连接：已安装服务为开机自启，未立即启动（避免抢锁）。重启后由服务接管。")
	}
	if err := clientapp.ServiceAutostartEnable(exe, startNow); err != nil {
		u.appendLog("启用服务失败: " + err.Error())
		return
	}
	if startNow {
		u.appendLog("已启用服务开机自启并尝试启动（无托盘；查看请再开本程序接管）")
	} else {
		u.appendLog("已启用服务开机自启")
	}
	u.refreshTrayMenu()
}

func (u *uiApp) maybeAutoConnect() {
	if u.cfg == nil || !u.cfg.CanAutoConnect() {
		return
	}
	if !u.requireAdmin("", func(string) { logger.Info("gui_auto_connect skip: not admin") }) {
		return
	}
	// begin 日志在 run.go（含 warmup_overlap）；此处直接拨号。
	u.tryConnect()
}

func (u *uiApp) ensureConfigLoaded() {
	if u.cfg == nil {
		u.cfg = u.loadOrCreateConfig()
		_ = u.cfg.Log.InitGlobal()
	}
}

func absPath(p string) (string, error) {
	if p == "" {
		return "", os.ErrInvalid
	}
	return filepath.Abs(p)
}
