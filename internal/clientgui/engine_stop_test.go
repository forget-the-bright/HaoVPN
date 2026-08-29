package clientgui

import (
	"testing"

	"haovpn/internal/clientapp"
)

// TestBeginEngineOpPreventsReentry 忙状态时二次 begin 失败，end 后可再 begin。
func TestBeginEngineOpPreventsReentry(t *testing.T) {
	u := &uiApp{}
	if !u.beginEngineOp() {
		t.Fatal("首次应成功")
	}
	if u.beginEngineOp() {
		t.Fatal("忙时不得再 begin")
	}
	u.endEngineOp()
	if !u.beginEngineOp() {
		t.Fatal("end 后应可再 begin")
	}
}

// TestTakeEngineClearsField takeEngine 取出后 eng 应为 nil。
func TestTakeEngineClearsField(t *testing.T) {
	u := &uiApp{}
	if u.takeEngine() != nil {
		t.Fatal("空 eng 应 nil")
	}
}

// TestSetGetEngineLocked set/get/clearEngineIf 与 take 一致。
func TestSetGetEngineLocked(t *testing.T) {
	u := &uiApp{}
	fake := &clientapp.Engine{}
	u.setEngine(fake)
	if u.getEngine() != fake {
		t.Fatal("get 应返回 set 的实例")
	}
	if !u.isSameEngine(fake) {
		t.Fatal("isSameEngine")
	}
	if !u.clearEngineIf(fake) || u.getEngine() != nil {
		t.Fatal("clearEngineIf 应清空")
	}
}

