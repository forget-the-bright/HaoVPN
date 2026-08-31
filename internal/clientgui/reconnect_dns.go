package clientgui

import (
	"fyne.io/fyne/v2"

	"haovpn/internal/clientapp"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
)

// reconnectVPNAfterStop 在旧引擎已 take 后调度 HardRestart（Stop+DNS settle+新 Engine）。
//
// 调用方：reconnectVPN 在 UI 线程 beginEngineOp/takeEngine 之后调用本函数。
// 为何再启后台：HardRestart 含 Stop/ICS/DNS，禁止在 fyne.Do 内阻塞；完成后 UI 经 fyne.Do 挂载。
// 契约唯一源：clientapp.HardRestart（禁止 GUI 再手写 NewEngine+settle）。
func (u *uiApp) reconnectVPNAfterStop(old *clientapp.Engine, creds clientapp.Credentials) {
	cfg := u.cfg
	safeutil.GoSafe("gui-hard-restart", func() {
		eng, err := clientapp.HardRestart(old, cfg, creds)
		fyne.Do(func() {
			if eng != nil {
				u.setEngine(eng)
			}
			if err != nil {
				logger.Warn("gui hard_restart start: %v", err)
				u.appendLog("重新连接启动失败: " + err.Error())
				u.applyTray(trayKindError, true)
			} else {
				u.startPoll()
				u.applyTray(trayKindConnecting, true)
			}
			u.endEngineOp()
		})
	})
}
