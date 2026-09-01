package clientgui

import "haovpn/internal/clientapp"

// engineOpQueue 引擎操作队列的纯状态机（无 Fyne），表驱动单测与 uiApp 共用同一套规则。
//
// 不变量：
//   - 仅 busy 时可登记 pending；
//   - logout/quit 优先于 reconnect（已排队终止意图时 reconnect 不降级）；
//   - 登记抢占意图时 bump OpGen，使 HardRestart abort；
//   - shouldAbort：世代过期或 pending 为 logout/quit。
//
// 关联：engine_intent.go（uiApp 加锁后委托本类型）；reconnect_dns.go finishHardRestart*。
type engineOpQueue struct {
	Busy          bool
	OpGen         uint64
	PendingIntent engineIntent
	PendingCreds  clientapp.Credentials
}

// bumpOpGen 递增世代（调用方须已保证并发安全）。
func (q *engineOpQueue) bumpOpGen() {
	q.OpGen++
}

// setPending 在 busy 时登记后续意图；非 busy 返回 false。
//
// logout/quit 优先：已排队终止意图时再登记 reconnect 仍返回 true 但不改写 intent。
func (q *engineOpQueue) setPending(intent engineIntent, creds clientapp.Credentials) bool {
	if intent == intentNone || !q.Busy {
		return false
	}
	if q.PendingIntent == intentQuit || q.PendingIntent == intentLogout {
		if intent == intentReconnect {
			return true
		}
	}
	if intent == intentReconnect {
		q.PendingCreds = creds
		q.bumpOpGen()
	} else {
		q.bumpOpGen()
		q.PendingCreds = clientapp.Credentials{}
	}
	q.PendingIntent = intent
	return true
}

// takePending 取出并清空排队意图。
func (q *engineOpQueue) takePending() (engineIntent, clientapp.Credentials) {
	it := q.PendingIntent
	creds := q.PendingCreds
	q.PendingIntent = intentNone
	q.PendingCreds = clientapp.Credentials{}
	return it, creds
}

// shouldAbort HardRestart 间隙是否应中止（世代过期或 pending 为退出类）。
func (q *engineOpQueue) shouldAbort(gen uint64) bool {
	if q.OpGen != gen {
		return true
	}
	switch q.PendingIntent {
	case intentLogout, intentQuit:
		return true
	default:
		return false
	}
}

// decideHardRestartFinish 纯函数：HardRestart UI 收口分支（无副作用）。
//
// 返回值 kind：
//   - "superseded"：世代被抢占，须带 orphan 消化 pending；
//   - "aborted"：ErrHardRestartAborted；
//   - "start_fail"：Start 失败；
//   - "mount"：成功挂载（pending 由 finishHardRestartUI 内 takePendingIntent 处理）。
func decideHardRestartFinish(genCurrent bool, aborted, startFail bool) string {
	if !genCurrent {
		return "superseded"
	}
	if aborted {
		return "aborted"
	}
	if startFail {
		return "start_fail"
	}
	return "mount"
}
