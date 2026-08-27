package clientapp

import (
	"fmt"

	"haovpn/internal/config"
	"haovpn/internal/credentials"
)

// ResolveCredentials 按优先级解析 CLI/Windows 服务启动所需的 VPN 登录凭据。
//
// 解析顺序：config.ClientConfig（yaml auth 与环境变量 HAOVPN_*）→ Windows 服务 DPAPI 凭据
//（credentials.LoadService）→ 终端 PromptPassword（仅当仍缺密码时）。
//
// 参数：cfg — 非 nil；须已通过 config 加载与 ApplyDefaults。
// 返回：user、password — 均非空时成功；err — 无用户名、DPAPI 读取失败、或无法获取密码。
// 副作用：缺密码时可能向 stderr 提示并阻塞读取终端一行（PromptPassword）。
// 并发：无内部锁；终端读取不可并行，调用方应单 goroutine 在 Start 前调用一次。
func ResolveCredentials(cfg *config.ClientConfig) (user, password string, err error) {
	user, password = cfg.ResolveAuth()
	if user == "" {
		cu, cp, loadErr := credentials.LoadService()
		if loadErr != nil {
			return "", "", fmt.Errorf("读取服务凭据失败: %w", loadErr)
		}
		if cu != "" && cp != "" {
			user, password = cu, cp
		}
	}
	if user == "" {
		return "", "", fmt.Errorf("请配置 auth.username 或保存服务凭据")
	}
	if password == "" {
		password, err = PromptPassword()
		if err != nil || password == "" {
			return "", "", fmt.Errorf("需要密码才能连接")
		}
	}
	return user, password, nil
}
