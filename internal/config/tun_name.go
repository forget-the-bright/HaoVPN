package config

import (
	"fmt"
	"regexp"
	"strings"
)

// tunNamePattern 限制 TUN/Wintun 适配器名字符集与长度。
//
// 为何收紧：名称会进入 netsh/PowerShell（含 -match）与 Remove-NetAdapter；
// 禁止空格、路径分隔符与正则元字符混入，降低注入与误删面。
var tunNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidateTunName 校验 TUN 网卡名是否允许写入配置并下发到 Windows 网卡/脚本。
//
// 参数：name — 通常来自 client.yaml tun.name（ApplyDefaults 后非空）。
// 返回：非法字符/长度时带字段说明的 error。
// 关联：ClientConfig.Validate；winnet PS 模板仍 EscapeRegex 作纵深防御。
func ValidateTunName(name string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return fmt.Errorf("tun.name 不能为空")
	}
	if !tunNamePattern.MatchString(n) {
		return fmt.Errorf("tun.name %q 非法：仅允许字母数字、下划线、连字符，长度 1～64", n)
	}
	return nil
}
