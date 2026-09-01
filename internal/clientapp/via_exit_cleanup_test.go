package clientapp

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// TestCleanupTUNAfterViaDisabledNoResidue 无残留时不得调用慢清理。
func TestCleanupTUNAfterViaDisabledNoResidue(t *testing.T) {
	var cleanupN, removeN atomic.Int32
	origHas, origClean, origRem := hasICSResidueFn, cleanupICSResidueFn, removeICSAddressesFn
	t.Cleanup(func() {
		hasICSResidueFn, cleanupICSResidueFn, removeICSAddressesFn = origHas, origClean, origRem
	})
	hasICSResidueFn = func(string) bool { return false }
	cleanupICSResidueFn = func(context.Context, string, string) error {
		cleanupN.Add(1)
		return nil
	}
	removeICSAddressesFn = func(string, string) error {
		removeN.Add(1)
		return nil
	}

	cleanupTUNAfterViaDisabled(context.Background(), "haovpn0", "10.88.0.2", false)
	cleanupTUNAfterViaDisabled(context.Background(), "haovpn0", "10.88.0.2", true)
	if cleanupN.Load() != 0 || removeN.Load() != 0 {
		t.Fatalf("无残留不应清理 cleanup=%d remove=%d", cleanupN.Load(), removeN.Load())
	}
}

// TestCleanupTUNAfterViaDisabledResidueFull 有残留且非 hadVia：走一次 CleanupICSResidueContext。
func TestCleanupTUNAfterViaDisabledResidueFull(t *testing.T) {
	var cleanupN, removeN atomic.Int32
	origHas, origClean, origRem := hasICSResidueFn, cleanupICSResidueFn, removeICSAddressesFn
	t.Cleanup(func() {
		hasICSResidueFn, cleanupICSResidueFn, removeICSAddressesFn = origHas, origClean, origRem
	})
	hasICSResidueFn = func(string) bool { return true }
	cleanupICSResidueFn = func(ctx context.Context, tun, vpn string) error {
		if ctx == nil {
			t.Fatal("ctx must not be nil after normalize")
		}
		if tun != "haovpn0" || vpn != "10.88.0.5" {
			t.Fatalf("args tun=%s vpn=%s", tun, vpn)
		}
		cleanupN.Add(1)
		return nil
	}
	removeICSAddressesFn = func(string, string) error {
		removeN.Add(1)
		return nil
	}

	cleanupTUNAfterViaDisabled(context.Background(), "haovpn0", "10.88.0.5", false)
	if cleanupN.Load() != 1 || removeN.Load() != 0 {
		t.Fatalf("want CleanupICSResidue only cleanup=%d remove=%d", cleanupN.Load(), removeN.Load())
	}
}

// TestCleanupTUNAfterViaDisabledHadVia 刚 Teardown 过：只清 137，不再全机 Disable。
func TestCleanupTUNAfterViaDisabledHadVia(t *testing.T) {
	var cleanupN, removeN atomic.Int32
	origHas, origClean, origRem := hasICSResidueFn, cleanupICSResidueFn, removeICSAddressesFn
	t.Cleanup(func() {
		hasICSResidueFn, cleanupICSResidueFn, removeICSAddressesFn = origHas, origClean, origRem
	})
	hasICSResidueFn = func(string) bool { return true }
	cleanupICSResidueFn = func(context.Context, string, string) error {
		cleanupN.Add(1)
		return nil
	}
	removeICSAddressesFn = func(string, string) error {
		removeN.Add(1)
		return nil
	}

	cleanupTUNAfterViaDisabled(context.Background(), "haovpn0", "10.88.0.5", true)
	if cleanupN.Load() != 0 || removeN.Load() != 1 {
		t.Fatalf("hadVia 应只 Remove addresses cleanup=%d remove=%d", cleanupN.Load(), removeN.Load())
	}
}

// TestCleanupTUNAfterViaDisabledWarnOnError 清理失败不 panic，仅走 Warn 路径。
func TestCleanupTUNAfterViaDisabledWarnOnError(t *testing.T) {
	origHas, origClean := hasICSResidueFn, cleanupICSResidueFn
	t.Cleanup(func() {
		hasICSResidueFn, cleanupICSResidueFn = origHas, origClean
	})
	hasICSResidueFn = func(string) bool { return true }
	cleanupICSResidueFn = func(context.Context, string, string) error { return errors.New("ps fail") }
	// 不应 panic
	cleanupTUNAfterViaDisabled(context.Background(), "haovpn0", "10.88.0.5", false)
}

// TestCleanupTUNAfterViaDisabledCanceled 取消时打 aborted 路径且不 panic。
func TestCleanupTUNAfterViaDisabledCanceled(t *testing.T) {
	origHas, origClean := hasICSResidueFn, cleanupICSResidueFn
	t.Cleanup(func() {
		hasICSResidueFn, cleanupICSResidueFn = origHas, origClean
	})
	hasICSResidueFn = func(string) bool { return true }
	cleanupICSResidueFn = func(context.Context, string, string) error { return context.Canceled }
	cleanupTUNAfterViaDisabled(context.Background(), "haovpn0", "10.88.0.5", false)
}

// TestSetupViaExitEmptySkipsCleanupWhenNoResidue 空 local_lans + 无残留：setup 不触发慢清理。
func TestSetupViaExitEmptySkipsCleanupWhenNoResidue(t *testing.T) {
	var cleanupN atomic.Int32
	origHas, origClean := hasICSResidueFn, cleanupICSResidueFn
	t.Cleanup(func() {
		hasICSResidueFn, cleanupICSResidueFn = origHas, origClean
	})
	hasICSResidueFn = func(string) bool { return false }
	cleanupICSResidueFn = func(context.Context, string, string) error {
		cleanupN.Add(1)
		return nil
	}

	rt := &runtime{}
	did, err := rt.setupViaExitLocked(context.Background(), "10.88.0.0/24", "haovpn0", "10.88.0.2", nil)
	if err != nil || did {
		t.Fatalf("empty lans: did=%v err=%v", did, err)
	}
	if !rt.viaFPKnown || rt.viaFP != "" {
		t.Fatalf("应标记 via 关闭已应用 fp=%q known=%v", rt.viaFP, rt.viaFPKnown)
	}
	if cleanupN.Load() != 0 {
		t.Fatal("无残留不应 CleanupICSResidue")
	}
	// 二次 apply：指纹未变，直接 skip
	did, err = rt.setupViaExitLocked(context.Background(), "10.88.0.0/24", "haovpn0", "10.88.0.2", nil)
	if err != nil || did || cleanupN.Load() != 0 {
		t.Fatalf("unchanged off did=%v err=%v cleanup=%d", did, err, cleanupN.Load())
	}
}
