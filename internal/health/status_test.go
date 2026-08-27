package health_test

import (
	"testing"
	"time"

	"haovpn/internal/health"
)

// TestNewStatusIncludesRecentErrors health Status 须携带 recent_errors。
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

func TestNewStatusNotOKWhenTunDown(t *testing.T) {
	st := health.NewStatus(time.Now(), 0, true, false, true, nil)
	if st.OK {
		t.Fatal("tun down must make ok=false")
	}
}
