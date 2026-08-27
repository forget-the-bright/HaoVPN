//go:build windows && amd64

package wintundll

import _ "embed"

//go:embed amd64/wintun.dll
var embeddedDLL []byte
