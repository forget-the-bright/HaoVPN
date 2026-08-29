package clientgui

import (
	"haovpn/internal/clientapp"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"

	"fyne.io/fyne/v2"
)

// beginEngineOp 标记正在停引擎/重连，防止按钮连点；已忙则返回 false。
//
// 与 getEngine/setEngine/takeEngine 共用 engOpMu，保证 eng 指针与忙状态一致可见。
func (u *uiApp) beginEngineOp() bool {
	u.engOpMu.Lock()
	defer u.engOpMu.Unlock()
	if u.engOpBusy {
		return false
	}
	u.engOpBusy = true
	return true
}

// endEngineOp 结束忙状态（须在 UI 线程或经 fyne.Do 调用以配合界面刷新）。
func (u *uiApp) endEngineOp() {
	u.engOpMu.Lock()
	u.engOpBusy = false
	u.engOpMu.Unlock()
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
func (u *uiApp) stopEngineAsync(eng *clientapp.Engine, onDone func()) {
	if eng == nil {
		if onDone != nil {
			onDone()
		}
		return
	}
	safeutil.GoSafe("gui-engine-stop", func() {
		logger.Info("gui_engine_stop begin")
		eng.Stop()
		logger.Info("gui_engine_stop done")
		if onDone == nil {
			return
		}
		fyne.Do(onDone)
	})
}
