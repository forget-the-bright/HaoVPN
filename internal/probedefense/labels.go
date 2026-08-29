package probedefense

// 特征 / 阶段 / 动作中文标签（与 docs/security-hardening.md 对照表同源；改码须同步文档）。

var signatureLabels = map[string]string{
	"account_online":       "账号已在其他设备在线",
	"auth_failed":          "用户名或密码错误",
	"source_deny":          "来源 IP 不在白名单",
	"handshake_reject":     "握手被拒绝",
	"http_get":             "HTTP GET 探测",
	"http_method":          "HTTP 方法探测",
	"http_blank":           "HTTP 空行探测",
	"amqp":                 "AMQP 协议扫描",
	"jrmi":                 "Java RMI 扫描",
	"giop":                 "CORBA/GIOP 扫描",
	"conn_probe":           "CONN 探测",
	"help_probe":           "HELP 探测",
	"nested_tls":           "套娃 TLS 探测",
	"frame_invalid":        "非法帧/未知协议",
	"sslv2":                "SSLv2 握手探测",
	"tls_bad_record":       "非 TLS 首包/坏记录",
	"tls_cipher_mismatch":  "TLS 密码套件不匹配",
	"tls_old_version":      "TLS 版本过旧",
	"tls_error":            "其它 TLS 错误",
	"connection_reset":     "对端重置连接",
	"unexpected_eof":       "对端提前断开",
	"banned":               "命中封禁",
	"manual":               "手动封禁",
	"unknown":              "未知",
}

var phaseLabels = map[string]string{
	PhaseTCPAccept: "TCP 接入",
	PhaseTLS:       "TLS 层",
	PhaseFrame:     "应用帧",
	PhaseHandshake: "账号握手",
	PhaseBanHit:    "封禁命中",
}

var actionLabels = map[string]string{
	ActionRejected:     "已拒绝",
	ActionBannedHit:    "撞上封禁",
	ActionAutoBanned:   "已自动封禁",
	ActionManualBanned: "已手动封禁",
}

// SignatureLabel 返回特征码中文；未知码原样返回。
func SignatureLabel(code string) string {
	if zh, ok := signatureLabels[code]; ok {
		return zh
	}
	return code
}

// PhaseLabel 返回阶段码中文；未知码原样返回。
func PhaseLabel(code string) string {
	if zh, ok := phaseLabels[code]; ok {
		return zh
	}
	return code
}

// ActionLabel 返回动作码中文；未知码原样返回。
func ActionLabel(code string) string {
	if zh, ok := actionLabels[code]; ok {
		return zh
	}
	return code
}

// FormatCodeZH 展示用「中文（英文码）」；空码返回空串。
func FormatCodeZH(code, zh string) string {
	if code == "" {
		return ""
	}
	if zh == "" || zh == code {
		return code
	}
	return zh + " (" + code + ")"
}
