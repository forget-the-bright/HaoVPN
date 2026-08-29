package auth

import (
	"fmt"
	"regexp"
	"strings"
)

// 用户名：1～64 字符，字母数字与 ._-；禁止纯空白。
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

// ValidateUsername 校验管理/VPN 账号登录名格式。
//
// 返回：空、空白或非法字符时 error（中文说明）；合法时 nil。
// 关联：api 开户、vpnaccount.ProvisionWebAccount 入口。
func ValidateUsername(username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("用户名不能为空")
	}
	if !usernamePattern.MatchString(username) {
		return fmt.Errorf("用户名仅允许字母数字与 ._-，长度 1～64")
	}
	return nil
}
