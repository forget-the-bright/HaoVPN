package vpnaccount

import "errors"

// ErrAccountNotFound 表示 users.id 不存在或账号无 VPN 隧道身份。
var ErrAccountNotFound = errors.New("账号不存在或无 VPN 身份")
