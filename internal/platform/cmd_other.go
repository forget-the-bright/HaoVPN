//go:build !windows

package platform

import (
	"context"
	"os/exec"
)

// Command 非 Windows：与 os/exec.Command 相同。
func Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}

// CommandContext 非 Windows：与 os/exec.CommandContext 相同。
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}
