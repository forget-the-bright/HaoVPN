package clientgui

import (
	"testing"

	"haovpn/internal/clientapp"
)

// TestEngineOpQueueSetPendingRules 表驱动：busy/优先级/bump。
func TestEngineOpQueueSetPendingRules(t *testing.T) {
	q := &engineOpQueue{}
	if q.setPending(intentLogout, clientapp.Credentials{}) {
		t.Fatal("非 busy 不得登记")
	}
	q.Busy = true
	if !q.setPending(intentReconnect, clientapp.Credentials{Username: "a"}) {
		t.Fatal("busy 应可 reconnect")
	}
	if q.OpGen != 1 || q.PendingIntent != intentReconnect {
		t.Fatalf("gen=%d intent=%v", q.OpGen, q.PendingIntent)
	}
	if !q.setPending(intentLogout, clientapp.Credentials{}) {
		t.Fatal("应可改登出")
	}
	if q.PendingIntent != intentLogout {
		t.Fatal("logout")
	}
	gen := q.OpGen
	if !q.setPending(intentReconnect, clientapp.Credentials{Username: "x"}) {
		t.Fatal("已 logout 时 reconnect 仍 true")
	}
	if q.PendingIntent != intentLogout || q.OpGen != gen {
		t.Fatal("logout 优先且勿因 reconnect 再 bump（已终止）")
	}
}

// TestEngineOpQueueShouldAbort 世代过期与 pending quit。
func TestEngineOpQueueShouldAbort(t *testing.T) {
	q := &engineOpQueue{Busy: true, OpGen: 3}
	if !q.shouldAbort(2) {
		t.Fatal("旧 gen 应 abort")
	}
	if q.shouldAbort(3) {
		t.Fatal("无 pending 时当前 gen 不 abort")
	}
	q.PendingIntent = intentQuit
	if !q.shouldAbort(3) {
		t.Fatal("pending quit 应 abort")
	}
}

// TestDecideHardRestartFinish 分支表。
func TestDecideHardRestartFinish(t *testing.T) {
	cases := []struct {
		name              string
		cur, abort, fail  bool
		want              string
	}{
		{"superseded", false, false, false, "superseded"},
		{"aborted", true, true, false, "aborted"},
		{"start_fail", true, false, true, "start_fail"},
		{"mount", true, false, false, "mount"},
	}
	for _, tc := range cases {
		got := decideHardRestartFinish(tc.cur, tc.abort, tc.fail)
		if got != tc.want {
			t.Fatalf("%s: got=%s want=%s", tc.name, got, tc.want)
		}
	}
}

// TestFinishLoginFailureBusyQueuesLogout beginEngineOp 失败时应排队 logout。
func TestFinishLoginFailureBusyQueuesLogout(t *testing.T) {
	u := &uiApp{}
	if !u.beginEngineOp() {
		t.Fatal("begin")
	}
	// 不依赖 Fyne 控件：仅验证 pending
	u.finishLoginFailure(nil, "鉴权失败")
	it, _ := u.takePendingIntent()
	if it != intentLogout {
		t.Fatalf("want intentLogout got=%v", it)
	}
	u.endEngineOp()
}
