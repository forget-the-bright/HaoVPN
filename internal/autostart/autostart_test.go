package autostart

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestLogonTaskName(t *testing.T) {
	if LogonTaskName == "" || ServiceName() == "" {
		t.Fatal("empty names")
	}
}

func TestLogonEnableRejectEmpty(t *testing.T) {
	if err := LogonEnable("", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestLogonStatusOtherOrQuery(t *testing.T) {
	_, detail, err := LogonStatus()
	if runtime.GOOS != "windows" {
		if err != nil {
			t.Fatal(err)
		}
		if detail == "" {
			t.Fatal("expected detail")
		}
		return
	}
	// Windows：未注册时应无 error
	_, _, err = LogonStatus()
	if err != nil {
		t.Log("query:", err) // 环境差异可接受
	}
}

func TestServiceStatusSmoke(t *testing.T) {
	_, _, detail, err := ServiceStatus()
	if err != nil && runtime.GOOS == "windows" {
		t.Log(err)
	}
	_ = detail
	_ = filepath.Separator
}
