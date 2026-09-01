package clientapp_test

import (
	"strings"
	"testing"
	"time"

	"haovpn/internal/clientapp"
)

func TestSingleInstanceHint(t *testing.T) {
	t.Parallel()
	if h := clientapp.SingleInstanceHint(false); h == "" || !strings.Contains(h, "已在运行") {
		t.Fatalf("cli hint: %q", h)
	}
	if h := clientapp.SingleInstanceHint(true); h == "" || !strings.Contains(h, "服务") {
		t.Fatalf("service hint: %q", h)
	}
}

func TestDefaultRunOptions(t *testing.T) {
	t.Parallel()
	cli := clientapp.DefaultInteractiveRunOptions()
	if !cli.WarmupTun || !cli.FailFastFirst || cli.WaitFirstAuth != 45*time.Second {
		t.Fatalf("interactive opts: %+v", cli)
	}
	if cli.OnUserWarn == nil || cli.OnDataplaneFail == nil {
		t.Fatal("interactive callbacks nil")
	}
	svc := clientapp.DefaultServiceRunOptions()
	if !svc.WarmupTun || svc.FailFastFirst || svc.WaitFirstAuth != 0 {
		t.Fatalf("service opts: %+v", svc)
	}
	if svc.Mode != clientapp.BootstrapService {
		t.Fatalf("mode=%s", svc.Mode)
	}
}
