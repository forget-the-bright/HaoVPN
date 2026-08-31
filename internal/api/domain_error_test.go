package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"haovpn/internal/probedefense"
	"haovpn/internal/vpnaccount"
)

// TestWriteDomainErrorMapping 钉死领域哨兵 → HTTP 状态（第 24 轮收口）。
func TestWriteDomainErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{vpnaccount.ErrAccountNotFound, http.StatusNotFound},
		{vpnaccount.ErrPeerRouteNotFound, http.StatusNotFound},
		{vpnaccount.ErrViaNotVPN, http.StatusBadRequest},
		{vpnaccount.ErrLastAdmin, http.StatusBadRequest},
		{probedefense.ErrProbeGuardNotReady, http.StatusServiceUnavailable},
		{probedefense.ErrBanExempt, http.StatusBadRequest},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		writeDomainError(w, tc.err)
		if w.Code != tc.code {
			t.Fatalf("%v: status=%d want %d body=%s", tc.err, w.Code, tc.code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	writeDomainError(w, nil)
	if w.Body.Len() != 0 {
		t.Fatalf("nil err should not write body: %s", w.Body.String())
	}
}
