package credentials

import (
	"path/filepath"
	"strings"
	"testing"

	"haovpn/internal/brand"
)

// TestCredPathUsesProgramData 凭据路径须在 ProgramData\HaoVPN 下（服务与 GUI 共用）。
func TestCredPathUsesProgramData(t *testing.T) {
	p := CredPath()
	if !strings.Contains(strings.ToLower(p), strings.ToLower(brand.CredDirName)) {
		t.Fatalf("path=%s", p)
	}
	if filepath.Base(p) != "credentials" {
		t.Fatalf("base=%s", filepath.Base(p))
	}
}
