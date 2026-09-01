package clientgui

import (
	"fyne.io/fyne/v2"

	"haovpn/internal/clientapp"
	"haovpn/internal/config"
)

// engineIntent 引擎操作进行中时排队的后续意图（可抢占 HardRestart）。
//
// 背景：engOpBusy 期间若直接拒绝「退出登录/退出/再点重连」，用户会感觉
// 「连接停不掉、清理和拨号搅在一起」。busy 时写入 intent + bump opGen，
// HardRestart 的 abort 回调与完成回调据此取消拨号或改走登出/退出。
// 规则实现见 engineOpQueue（本文件仅加锁委托，避免与单测分叉）。
type engineIntent int

const (
	intentNone engineIntent = iota
	intentLogout
	intentQuit
	intentReconnect
)

// withOpQueue 在 engOpMu 下读写与 engOpBusy/opGen/pending 对齐的队列快照。
func (u *uiApp) withOpQueue(fn func(q *engineOpQueue)) {
	u.engOpMu.Lock()
	defer u.engOpMu.Unlock()
	q := engineOpQueue{
		Busy:          u.engOpBusy,
		OpGen:         u.opGen,
		PendingIntent: u.pendingIntent,
		PendingCreds:  u.pendingCreds,
	}
	fn(&q)
	u.engOpBusy = q.Busy
	u.opGen = q.OpGen
	u.pendingIntent = q.PendingIntent
	u.pendingCreds = q.PendingCreds
}

// readOpGen 读取当前操作世代（供 HardRestart 闭包捕获）。
func (u *uiApp) readOpGen() uint64 {
	u.engOpMu.Lock()
	defer u.engOpMu.Unlock()
	return u.opGen
}

// isOpGenCurrent gen 是否仍为当前世代（未被再点重连/登出抢占）。
func (u *uiApp) isOpGenCurrent(gen uint64) bool {
	u.engOpMu.Lock()
	defer u.engOpMu.Unlock()
	return u.opGen == gen
}

// setPendingIntent 在已 busy 时登记后续意图；logout/quit 优先于 reconnect。
//
// 返回 true 表示已登记（调用方应提示用户「将在清理后执行」）。
func (u *uiApp) setPendingIntent(intent engineIntent, creds clientapp.Credentials) bool {
	ok := false
	u.withOpQueue(func(q *engineOpQueue) {
		ok = q.setPending(intent, creds)
	})
	return ok
}

// takePendingIntent 取出并清空排队意图（须在 UI 线程、结束一阶段 op 时调用）。
func (u *uiApp) takePendingIntent() (engineIntent, clientapp.Credentials) {
	var it engineIntent
	var creds clientapp.Credentials
	u.withOpQueue(func(q *engineOpQueue) {
		it, creds = q.takePending()
	})
	return it, creds
}

// hardRestartAbortFn 供 HardRestart 在 Stop/DNS/Start 间隙查询是否应中止。
func (u *uiApp) hardRestartAbortFn(gen uint64) func() bool {
	return func() bool {
		abort := false
		u.withOpQueue(func(q *engineOpQueue) {
			abort = q.shouldAbort(gen)
		})
		return abort
	}
}

// applyPendingAfterEngineOp 在 Stop 收尾时执行排队意图（无未挂载 orphan）。
//
// 返回 true 表示已接管后续，调用方不要再 endEngineOp/finishLogout。
func (u *uiApp) applyPendingAfterEngineOp() bool {
	it, creds := u.takePendingIntent()
	if it == intentNone {
		return false
	}
	u.dispatchPendingWithOrphan(it, creds, nil)
	return true
}

// attachDataplaneHook 鉴权成功后数据面失败 → 回登录红字（HardRestart 挂载路径仍用此薄封装）。
func (u *uiApp) attachDataplaneHook(eng *clientapp.Engine) {
	if eng == nil {
		return
	}
	clientapp.AttachDataplaneHook(eng, u.dataplaneHookFor(eng))
}

// dataplaneHookFor 构造绑定 eng 实例的数据面失败回调（供 DefaultGUIRunOptions 与 attachDataplaneHook）。
func (u *uiApp) dataplaneHookFor(eng *clientapp.Engine) func(string) {
	return func(msg string) {
		fyne.Do(func() {
			if !u.isSameEngine(eng) {
				return
			}
			u.requestLogoutWithMessage(msg)
		})
	}
}

// prepareGUIEngine 登录/HardRestart 共用：PrepareEngine + DefaultGUIRunOptions 内挂 dataplane hook。
func (u *uiApp) prepareGUIEngine(cfg *config.ClientConfig, creds clientapp.Credentials) (*clientapp.Engine, error) {
	var mount *clientapp.Engine
	opts := clientapp.DefaultGUIRunOptions(func(msg string) {
		if mount == nil {
			return
		}
		u.dataplaneHookFor(mount)(msg)
	})
	eng, err := clientapp.PrepareEngine(cfg, creds, opts)
	if err == nil {
		mount = eng
	}
	return eng, err
}
