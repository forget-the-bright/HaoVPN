package sessionmgr_test

import (
	"testing"

	"haovpn/internal/sessionmgr"
)

// TestKickUserInvokesHandler 验证 KickUser 会触发踢线回调（禁用账号/改策略依赖此行为）。
func TestKickUserInvokesHandler(t *testing.T) {
	mgr := sessionmgr.New(nil)
	var got int64
	mgr.SetKickHandler(func(id int64) { got = id })
	mgr.KickUser(99)
	if got != 99 {
		t.Fatalf("KickUser 应调用 handler，got=%d", got)
	}
}
