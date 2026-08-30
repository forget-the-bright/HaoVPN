//go:build !windows

package autostart

import (
	"fmt"
	"time"
)

func serviceStatus() (bool, bool, string, error) {
	return false, false, "非 Windows 无 SCM 服务；请用 systemd/launchd", nil
}

func serviceEnable(exe string, startNow bool) error {
	return fmt.Errorf("非 Windows 不支持 SCM 服务自启，请配置 systemd（见 docs/deploy.md）")
}

func serviceDisable() error { return nil }

func serviceStopAndWait(timeout time.Duration) error { return nil }
