package winnet_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"haovpn/internal/winnet"
)

// TestRunPSOneShotContextCanceled 已取消的 ctx 须立即失败，勿起 powershell。
func TestRunPSOneShotContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := winnet.RunPSOneShotContext(ctx, "Write-Output hi")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want Canceled", err)
	}
}

// TestRunPSOneShotContextCancelDuringSleep 运行中取消须结束（Kill），不得拖满脚本 Sleep。
func TestRunPSOneShotContextCancelDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := winnet.RunPSOneShotContext(ctx, "Start-Sleep -Seconds 30; Write-Output done")
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("取消后应返回 error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("取消后 powershell 未在 5s 内退出")
	}
}

// TestRunPSBestEffortContextCanceled 已取消 ctx 须快速返回（不抛 panic、不阻塞）。
func TestRunPSBestEffortContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	winnet.RunPSBestEffortContext(ctx, "Start-Sleep -Seconds 30", "test_best_effort")
	if time.Since(start) > 2*time.Second {
		t.Fatalf("BestEffort 已取消仍阻塞 elapsed=%s", time.Since(start))
	}
}

// TestDisableAllICSContextCanceled 已取消时 DisableAllICSContext 须立即返回。
func TestDisableAllICSContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	winnet.DisableAllICSContext(ctx)
	if time.Since(start) > 2*time.Second {
		t.Fatalf("DisableAllICS 已取消仍阻塞 elapsed=%s", time.Since(start))
	}
}
