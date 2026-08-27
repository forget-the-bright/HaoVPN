//go:build windows && arm64

package wintundll

import _ "embed"

//go:embed arm64/wintun.dll
var embeddedDLL []byte
