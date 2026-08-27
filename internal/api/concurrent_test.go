package api_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

// TestConcurrentHealthAPI 并发请求 health/dashboard 应全部 200（服务端 HTTP 路径）。
func TestConcurrentHealthAPI(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "api-conc.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecurePass123!")
	cfg := testServerCfg()
	pool, _ := ippool.New(cfg.VPN.Subnet)
	pool.Reserve(cfg.VPN.GatewayIP)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "pk")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	const workers = 32
	const perWorker = 10
	var wg sync.WaitGroup
	var bad atomic.Int32

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				resp, err := http.Get(ts.URL + "/api/v1/health")
				if err != nil || resp.StatusCode != http.StatusOK {
					bad.Add(1)
					if resp != nil {
						resp.Body.Close()
					}
					continue
				}
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()

	if bad.Load() > 0 {
		t.Fatalf("health 并发失败 %d 次", bad.Load())
	}
}
