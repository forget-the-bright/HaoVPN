package clientgui

import (
	"errors"

	"fyne.io/fyne/v2"

	"haovpn/internal/clientapp"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
)

// reconnectVPNAfterStop 在旧引擎已 take 后调度 HardRestart（Stop+DNS settle+新 Engine）。
//
// 调用方：reconnectVPN / pending reconnect 在仍 engOpBusy 时调用。
// abort：opGen 变化或 pending logout/quit 时中止 DNS/Start。
// Start 失败仍可能返回 eng，须 Stop，禁止 setEngine 挂僵尸。
func (u *uiApp) reconnectVPNAfterStop(old *clientapp.Engine, creds clientapp.Credentials) {
	cfg := u.cfg
	gen := u.readOpGen()
	abort := u.hardRestartAbortFn(gen)
	safeutil.GoSafe("gui-hard-restart", func() {
		eng, err := clientapp.HardRestart(old, cfg, creds, abort)
		fyne.Do(func() {
			u.finishHardRestartUI(eng, err, gen)
		})
	})
}

// finishHardRestartUI HardRestart 结束后的 UI 收口（须在 UI 线程；调用时仍 engOpBusy）。
//
// 分支与 decideHardRestartFinish 对齐，便于单测纯逻辑、本函数只做副作用。
func (u *uiApp) finishHardRestartUI(eng *clientapp.Engine, err error, gen uint64) {
	aborted := errors.Is(err, clientapp.ErrHardRestartAborted)
	startFail := err != nil && !aborted
	// 先 peek pending 会破坏队列；superseded/aborted/fail 走 orphan 路径内 take
	kind := decideHardRestartFinish(u.isOpGenCurrent(gen), aborted, startFail)
	if kind == "superseded" {
		logger.Info("gui_hard_restart superseded gen=%d", gen)
		u.finishHardRestartWithOrphan(eng)
		return
	}
	if kind == "aborted" {
		logger.Info("gui_hard_restart aborted")
		u.finishHardRestartWithOrphan(nil)
		return
	}
	if kind == "start_fail" {
		userMsg := clientapp.FormatConnectFailure(err, "", nil)
		if userMsg == "" {
			userMsg = clientapp.FormatDialError(err)
		}
		logger.Warn("gui hard_restart start: %v", err)
		u.appendLog("重新连接启动失败: " + userMsg)
		u.setTrayStickyErr(userMsg)
		u.applyTray(trayKindError, true)
		u.finishHardRestartWithOrphan(eng)
		return
	}

	// mount 或 pending：取 pending 再定
	it, creds := u.takePendingIntent()
	if it != intentNone {
		logger.Info("gui_hard_restart pending_after_start intent=%d", it)
		u.dispatchPendingWithOrphan(it, creds, eng)
		return
	}

	u.clearTrayStickyErr()
	u.attachDataplaneHook(eng)
	u.setEngine(eng)
	u.endEngineOp()
	u.startPoll()
	u.applyTray(trayKindConnecting, true)
	if u.statusLbl != nil {
		u.statusLbl.SetText("状态: 连接中…")
	}
	logger.Info("gui_hard_restart mounted")
}

// finishHardRestartWithOrphan 消化 pending；orphan 为已 Start 但未挂载的引擎（须 Stop）。
func (u *uiApp) finishHardRestartWithOrphan(orphan *clientapp.Engine) {
	it, creds := u.takePendingIntent()
	if it != intentNone {
		u.dispatchPendingWithOrphan(it, creds, orphan)
		return
	}
	if orphan != nil {
		u.stopEngineAsync(orphan, func() {
			u.endEngineOp()
			u.applyTray(trayKindError, true)
			logger.Info("gui_hard_restart_fail cleanup_done")
		})
		return
	}
	u.endEngineOp()
}

// dispatchPendingWithOrphan 执行排队意图；orphan 若非 nil 须先 Stop 再登出/再重连/退出。
func (u *uiApp) dispatchPendingWithOrphan(it engineIntent, creds clientapp.Credentials, orphan *clientapp.Engine) {
	switch it {
	case intentLogout:
		u.appendLog("正在退出登录…")
		logger.Info("gui_pending intent=logout")
		u.stopPoll()
		u.applyTray(trayKindDisconnecting, true)
		mounted := u.takeEngine()
		u.stopEnginesSerial([]*clientapp.Engine{mounted, orphan}, func() {
			u.endEngineOp()
			u.finishLogoutUI()
		})
	case intentQuit:
		u.appendLog("正在退出…")
		logger.Info("gui_pending intent=quit")
		u.stopPoll()
		u.applyTray(trayKindDisconnecting, true)
		mounted := u.takeEngine()
		u.stopEnginesSerial([]*clientapp.Engine{mounted, orphan}, func() {
			u.finishQuitApp()
		})
	case intentReconnect:
		u.appendLog("正在按新的重新连接请求重拨…")
		logger.Info("gui_pending intent=reconnect")
		u.stopPoll()
		u.applyTray(trayKindDisconnecting, true)
		mounted := u.takeEngine()
		// 串行：先停 mounted/orphan，再 HardRestart(nil)（旧面已清）；保持 busy
		u.stopEnginesSerial([]*clientapp.Engine{mounted, orphan}, func() {
			u.reconnectVPNAfterStop(nil, creds)
		})
	default:
		if orphan != nil {
			u.stopEngineAsync(orphan, func() { u.endEngineOp() })
			return
		}
		u.endEngineOp()
	}
}
