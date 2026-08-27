//go:build windows

package platform

import "testing"

func TestCommandHidesWindow(t *testing.T) {
	cmd := Command("cmd", "/c", "exit", "0")
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow false")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatal("missing CREATE_NO_WINDOW")
	}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}
