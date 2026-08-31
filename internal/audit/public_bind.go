package audit

// LogPublicBindEnabled 公网绑定启动时写审计记录（serverapp 启动阶段调用，不依赖 api 包）。
func LogPublicBindEnabled(auditLog *Logger) {
	auditLog.Log(nil, "management_public_bind_enabled", "system", nil, "", map[string]string{
		"message": "用户已开启 allow_public_bind，管理口暴露风险自担",
	})
}
