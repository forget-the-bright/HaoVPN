package singleinstance_test

import (
	"errors"
	"testing"
	"time"

	"haovpn/internal/singleinstance"
)

func TestAcquireClientExclusive(t *testing.T) {
	a, err := singleinstance.AcquireClient()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer a.Release()

	if !singleinstance.ClientAlreadyRunning() {
		t.Fatal("ClientAlreadyRunning should be true while lock held")
	}

	_, err = singleinstance.AcquireClient()
	if !errors.Is(err, singleinstance.ErrAlreadyRunning) {
		t.Fatalf("second acquire want ErrAlreadyRunning, got %v", err)
	}
}

func TestClientAlreadyRunningFalseWhenFree(t *testing.T) {
	if singleinstance.ClientAlreadyRunning() {
		t.Fatal("expected no running instance")
	}
}

func TestReleaseAllowsReacquire(t *testing.T) {
	lock, err := singleinstance.AcquireClient()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	lock.Release()
	time.Sleep(50 * time.Millisecond)
	if singleinstance.ClientAlreadyRunning() {
		t.Fatal("expected instance free after release")
	}
	lock2, err := singleinstance.AcquireClient()
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	lock2.Release()
}

func TestAlreadyRunningMessage(t *testing.T) {
	msg := singleinstance.AlreadyRunningMessage()
	if msg == "" {
		t.Fatal("empty message")
	}
}

func TestClientAlreadyRunning(t *testing.T) {
	lock, err := singleinstance.AcquireClient()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer lock.Release()
	if !singleinstance.ClientAlreadyRunning() {
		t.Fatal("ClientAlreadyRunning should be true while lock held")
	}
}

// TestProbeBeforeUACScenario 模拟 GUI：已有实例 Listen 时，新进程 Probe 应成功（无需抢锁）。
func TestProbeBeforeUACScenario(t *testing.T) {
	holder, err := singleinstance.AcquireClient()
	if err != nil {
		t.Fatalf("holder acquire: %v", err)
	}
	defer holder.Release()

	// 新「进程」仅 Probe，等价于 UAC 前的 ClientAlreadyRunning。
	if !singleinstance.ClientAlreadyRunning() {
		t.Fatal("probe should detect running instance before UAC")
	}
}
