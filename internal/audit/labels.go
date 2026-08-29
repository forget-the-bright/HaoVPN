package audit

// 管理审计动作 / 目标类型中文标签。
// 与 docs/security-hardening.md「管理审计动作/目标字典」对照表同源；新增 audit.Log 动作时须同步本文件与文档。

var actionLabels = map[string]string{
	"login":                           "登录",
	"login_failed":                    "登录失败",
	"logout":                          "退出登录",
	"change_password":                 "修改密码",
	"account_create":                  "创建账号",
	"account_delete":                  "删除账号",
	"user_enable":                     "启用账号",
	"user_disable":                    "禁用账号",
	"kick_account":                    "踢线",
	"admin_reset_password":            "管理员重置密码",
	"policy_change_kick":              "策略变更踢线",
	"config_export":                   "导出客户端配置",
	"db_backup":                       "数据库备份",
	"management_public_bind_enabled":  "管理口公网绑定已开启",
	"peer_route_create":               "创建托管路由",
	"peer_route_delete":               "删除托管路由",
	"peer_route_members":              "更新托管路由访问方",
	"peers_apply":                     "应用托管路由",
	"peer_access_add":                 "添加互访白名单",
	"peer_access_remove":              "移除互访白名单",
	"vpn_peers_policy":                "全局互访策略",
	"probe_ban_manual":                "手动封禁 IP",
	"probe_unban":                     "解封 IP",
}

var targetTypeLabels = map[string]string{
	"user":        "用户",
	"system":      "系统",
	"peer_route":  "托管路由",
	"peer_policy": "互访策略",
	"security":    "安全策略",
	"ip":          "IP",
}

// ActionLabel 返回动作码中文；未知码返回空串（展示层可仅显示英文码）。
func ActionLabel(code string) string {
	if zh, ok := actionLabels[code]; ok {
		return zh
	}
	return ""
}

// TargetTypeLabel 返回目标类型中文；未知码返回空串。
func TargetTypeLabel(code string) string {
	if zh, ok := targetTypeLabels[code]; ok {
		return zh
	}
	return ""
}

// FormatActionZH 展示用「英文码（中文）」；无中文时仅返回英文码。
func FormatActionZH(code string) string {
	if code == "" {
		return ""
	}
	zh := ActionLabel(code)
	if zh == "" {
		return code
	}
	return code + "（" + zh + "）"
}
