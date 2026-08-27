//go:build !windows

package platform

import "os/exec"

// Command 非 Windows：与 os/exec.Command 相同。
func Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}
