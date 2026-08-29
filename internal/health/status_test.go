package health_test

import (
	"testing"
	"time"

	"haovpn/internal/health"
)

// TestNewStatusIncludesRecentErrors NewStatus 可携带 recent_errors（供需鉴权的 Dashboard）。
func TestNewStatusIncludesRecentErrors(t *testing.T) {
	recent := []string{"[WARN] sample", "[ERROR] boom"}
	st := health.NewStatus(time.Now().Add(-time.Minute), 2, true, true, true, recent)
	if !st.OK {
		t.Fatal("expected ok")
	}
	if len(st.RecentErrors) != 2 {
		t.Fatalf("recent_errors: %v", st.RecentErrors)
	}
	if st.OnlinePeers != 2 || !st.TunOK || !st.NatOK {
		t.Fatalf("%+v", st)
	}
}

// TestNewStatusOmitsRecentWhenNil 公开 health 传 nil 时 JSON 不应带 recent_errors（omitempty）。
func TestNewStatusOmitsRecentWhenNil(t *testing.T) {
	st := health.NewStatus(time.Now(), 0, true, true, true, nil)
	if len(st.RecentErrors) != 0 {
		t.Fatalf("expected empty, got %v", st.RecentErrors)
	}
}

func TestNewStatusNotOKWhenTunDown(t *testing.T) {
	st := health.NewStatus(time.Now(), 0, true, false, true, nil)
	if st.OK {
		t.Fatal("tun down must make ok=false")
	}
}
