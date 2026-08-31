package probedefense

import (
	"errors"

	"haovpn/internal/autherr"
)

// ErrBanExempt 手动封禁时 IP 在豁免名单。
var ErrBanExempt = errors.New("该 IP 在封禁豁免名单")

// ErrInvalidBanIP 手动封禁时 IP 非法（非可解析单 IP）。
var ErrInvalidBanIP = errors.New("无效 IP 地址")

// ErrProbeGuardNotReady 探针 Guard 未注入时 API 拒绝封禁写操作。
var ErrProbeGuardNotReady = errors.New("探针防御未就绪")

// ErrSourceDenied 隧道来源白名单拒绝；定义在 autherr，此处 re-export 保持 import 路径稳定。
var ErrSourceDenied = autherr.ErrSourceDenied
