package clientgui

import (
	"testing"

	"haovpn/internal/clientapp"
)

// TestSetPendingIntentWhileBusy 忙态可登记 logout/reconnect，且 reconnect bump opGen。
func TestSetPendingIntentWhileBusy(t *testing.T) {
	u := &uiApp{}
	if u.setPendingIntent(intentLogout, clientapp.Credentials{}) {
		t.Fatal("未 busy 不得登记")
	}
	if !u.beginEngineOp() {
		t.Fatal("begin")
	}
	gen0 := u.readOpGen()
	if !u.setPendingIntent(intentReconnect, clientapp.Credentials{Username: "a"}) {
		t.Fatal("busy 应可登记 reconnect")
	}
	if u.readOpGen() == gen0 {
		t.Fatal("reconnect 应 bump opGen")
	}
	if !u.hardRestartAbortFn(gen0)() {
		t.Fatal("旧 gen 应 abort")
	}
	it, creds := u.takePendingIntent()
	if it != intentReconnect || creds.Username != "a" {
		t.Fatalf("it=%v creds=%+v", it, creds)
	}
	u.endEngineOp()
}

// TestPendingLogoutPreemptsReconnect 已排队 logout 时再点 reconnect 不降级。
func TestPendingLogoutPreemptsReconnect(t *testing.T) {
	u := &uiApp{}
	if !u.beginEngineOp() {
		t.Fatal("begin")
	}
	if !u.setPendingIntent(intentLogout, clientapp.Credentials{}) {
		t.Fatal("logout")
	}
	if !u.setPendingIntent(intentReconnect, clientapp.Credentials{Username: "x"}) {
		t.Fatal("reconnect while logout pending should still return true")
	}
	it, _ := u.takePendingIntent()
	if it != intentLogout {
		t.Fatalf("logout 优先 got=%v", it)
	}
	u.endEngineOp()
}

// TestHardRestartAbortFnOnPendingQuit pending quit 时 abort 为 true。
func TestHardRestartAbortFnOnPendingQuit(t *testing.T) {
	u := &uiApp{}
	if !u.beginEngineOp() {
		t.Fatal("begin")
	}
	gen := u.readOpGen()
	_ = u.setPendingIntent(intentQuit, clientapp.Credentials{})
	if !u.hardRestartAbortFn(gen)() {
		// setPendingIntent bumps gen，故用旧 gen 也会 abort；用新 gen 也应因 pending quit abort
		t.Fatal("旧 gen 应 abort")
	}
	gen2 := u.readOpGen()
	if !u.hardRestartAbortFn(gen2)() {
		t.Fatal("pending quit 时当前 gen 亦应 abort")
	}
	u.endEngineOp()
}
