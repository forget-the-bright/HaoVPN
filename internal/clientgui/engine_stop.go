package clientgui

import (
	"haovpn/internal/clientapp"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// beginEngineOp 标记正在停引擎/重连，防止按钮连点；已忙则返回 false。
//
// 与 getEngine/setEngine/takeEngine 共用 engOpMu，保证 eng 指针与忙状态一致可见。
// 成功时灰掉主窗/登录操作钮（须在 UI 线程调用）。
func (u *uiApp) beginEngineOp() bool {
	u.engOpMu.Lock()
	if u.engOpBusy {
		u.engOpMu.Unlock()
		return false
	}
	u.engOpBusy = true
	u.engOpMu.Unlock()
	u.setEngineOpBusyUI(true)
	return true
}

// isEngineOpBusy 是否正在异步 Stop/重连（托盘 tip 用「正在断开」覆盖）。
func (u *uiApp) isEngineOpBusy() bool {
	u.engOpMu.Lock()
	defer u.engOpMu.Unlock()
	return u.engOpBusy
}

// endEngineOp 结束忙状态（须在 UI 线程或经 fyne.Do 调用以配合界面刷新）。
//
// 必须刷新托盘 tip：忙时 tip 被强制为「正在断开…」，若只清 busy 不刷 tip，
// lastTooltip 会一直卡在断开文案（退出登录回登录页的典型坑）。
func (u *uiApp) endEngineOp() {
	u.engOpMu.Lock()
	u.engOpBusy = false
	u.engOpMu.Unlock()
	u.setEngineOpBusyUI(false)
	u.refreshTrayTooltip()
}

// setEngineOpBusyUI 按忙态 Disable/Enable 主窗与登录操作钮（按钮可为 nil）。
func (u *uiApp) setEngineOpBusyUI(busy bool) {
	set := func(b *widget.Button) {
		if b == nil {
			return
		}
		if busy {
			b.Disable()
		} else {
			b.Enable()
		}
	}
	set(u.reconnectBtn)
	set(u.logoutBtn)
	set(u.quitBtn)
	set(u.connectBtn)
}

// getEngine 在锁保护下读取当前引擎指针（只读快照；返回后指针可能被他处 take）。
func (u *uiApp) getEngine() *clientapp.Engine {
	u.engOpMu.Lock()
	defer u.engOpMu.Unlock()
	return u.eng
}

// setEngine 在锁保护下写入引擎指针（登录成功/重连挂载新 Engine）。
func (u *uiApp) setEngine(eng *clientapp.Engine) {
	u.engOpMu.Lock()
	u.eng = eng
	u.engOpMu.Unlock()
}

// takeEngine 取出并清空 u.eng（调用方负责 Stop）；无引擎时返回 nil。
func (u *uiApp) takeEngine() *clientapp.Engine {
	u.engOpMu.Lock()
	defer u.engOpMu.Unlock()
	eng := u.eng
	u.eng = nil
	return eng
}

// clearEngineIf 若当前 eng 仍是指定实例则清空（登录失败/WaitConnected 竞态时避免误清新引擎）。
func (u *uiApp) clearEngineIf(eng *clientapp.Engine) bool {
	u.engOpMu.Lock()
	defer u.engOpMu.Unlock()
	if u.eng == eng {
		u.eng = nil
		return true
	}
	return false
}

// isSameEngine 判断当前挂载的是否仍为指定引擎实例。
func (u *uiApp) isSameEngine(eng *clientapp.Engine) bool {
	u.engOpMu.Lock()
	defer u.engOpMu.Unlock()
	return u.eng == eng
}

// stopEngineAsync 在后台 Stop（ICS/路由清理可能数秒），完成后在 UI 线程执行 onDone。
//
// 切勿在 Fyne UI 回调里同步 eng.Stop：DisableAllICS 的 PowerShell COM 会卡住界面。
// eng==nil 时 onDone 仍经 fyne.Do，避免调用方假定已在主线程（fyneDo=true 后更关键）。
func (u *uiApp) stopEngineAsync(eng *clientapp.Engine, onDone func()) {
	u.stopEnginesSerial([]*clientapp.Engine{eng}, onDone)
}

// stopEnginesSerial 按序后台 Stop 多个引擎（跳过 nil），全部完成后在 UI 线程 onDone。
//
// 用途：HardRestart 收口时同时有 mounted（takeEngine）与 orphan（已 Start 未挂载），
// 须串行清完再登出/退出/再拨，禁止嵌套复制 stopEngineAsync 回调链。
// 顺序：切片从前到后；nil 项跳过；全 nil 时仍 fyne.Do(onDone)。
// 关联：dispatchPendingWithOrphan、finishHardRestartWithOrphan。
func (u *uiApp) stopEnginesSerial(engs []*clientapp.Engine, onDone func()) {
	var chain []*clientapp.Engine
	for _, e := range engs {
		if e != nil {
			chain = append(chain, e)
		}
	}
	if len(chain) == 0 {
		if onDone != nil {
			fyne.Do(onDone)
		}
		return
	}
	var run func(i int)
	run = func(i int) {
		if i >= len(chain) {
			if onDone != nil {
				fyne.Do(onDone)
			}
			return
		}
		eng := chain[i]
		safeutil.GoSafe("gui-engine-stop", func() {
			logger.Info("gui_engine_stop begin i=%d/%d", i+1, len(chain))
			eng.Stop()
			logger.Info("gui_engine_stop done i=%d/%d", i+1, len(chain))
			fyne.Do(func() { run(i + 1) })
		})
	}
	run(0)
}

// networkOpKind 网络操作前奏类型（登出/退出/重连/登录清理）。
type networkOpKind int

const (
	networkOpLogout networkOpKind = iota
	networkOpQuit
	networkOpReconnect
	networkOpLoginCleanup
)

// beginNetworkOp 在 beginEngineOp 成功后统一：stopPoll、Disconnecting 托盘、按 kind 更新 UI、takeEngine。
//
// 调用方须已 beginEngineOp 成功；返回取出的旧引擎（可能 nil），由调用方 Stop 或 HardRestart。
func (u *uiApp) beginNetworkOp(kind networkOpKind) *clientapp.Engine {
	u.stopPoll()
	u.applyTray(trayKindDisconnecting, true)
	switch kind {
	case networkOpLogout:
		if u.statusLbl != nil {
			u.statusLbl.SetText("状态: 正在断开…")
		}
		u.appendLog("正在退出登录（清理网络可能需数秒）…")
	case networkOpQuit:
		if u.statusLbl != nil {
			u.statusLbl.SetText("状态: 正在退出（清理网络）…")
		}
		u.appendLog("正在退出（清理网络可能需数秒）…")
		logger.Info("gui_quit begin")
	case networkOpReconnect:
		if u.statusLbl != nil {
			u.statusLbl.SetText("状态: 正在重新连接…")
		}
		u.appendLog("手动重新连接（清理后重拨，可能需数秒）…")
		logger.Info("gui_reconnect begin")
	case networkOpLoginCleanup:
		if u.errLbl != nil {
			u.errLbl.SetText("正在清理上一连接…")
		}
	}
	return u.takeEngine()
}
