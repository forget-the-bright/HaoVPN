package clientapp

import (
	"fmt"
	"os"

	"golang.org/x/term"

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
// 副作用：缺密码时可能向 stderr 提示并阻塞读取终端（PromptPassword，无回显）。
// 并发：无内部锁；终端读取不可并行，调用方应单 goroutine 在 Start 前调用一次。
func ResolveCredentials(cfg *config.ClientConfig) (user, password string, err error) {
	user, password = cfg.ResolveAuth()
	// YAML 仅有用户名、密码为空时，仍尝试服务凭据库补密码（Windows 服务场景）。
	if user == "" || password == "" {
		cu, cp, loadErr := credentials.LoadService()
		if loadErr != nil && user == "" {
			return "", "", fmt.Errorf("读取服务凭据失败: %w", loadErr)
		}
		if loadErr == nil {
			if user == "" && cu != "" {
				user = cu
			}
			if password == "" && cp != "" {
				password = cp
			}
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

// PromptPassword 从终端读取密码（无回显）。
//
// 使用 golang.org/x/term.ReadPassword；stdin 非 TTY 时返回错误，提示改用
// HAOVPN_PASSWORD 或配置文件，避免明文回显。
//
// 返回：输入的密码字符串；err 为非 TTY、读取失败或用户取消。
// 副作用：向 stderr 打印提示；阻塞直到回车。
func PromptPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("当前不是交互终端，请设置 HAOVPN_PASSWORD 或在配置中填写 auth.password")
	}
	fmt.Fprint(os.Stderr, "请输入密码（或设置 HAOVPN_PASSWORD）: ")
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr) // 无回显输入后补换行，避免后续日志贴在同一行
	if err != nil {
		return "", err
	}
	return string(b), nil
}
