package auth

// MustChangePassword 查询用户是否仍须首次改密。
//
// 参数：userID — persist.User 主键。
// 返回：须改密为 true；用户不存在时 err 非 nil。
// 副作用：只读 users 表。
func (s *Service) MustChangePassword(userID int64) (bool, error) {
	u, err := s.store.GetUserByID(userID)
	if err != nil {
		return false, err
	}
	return u.MustChangePassword, nil
}

// setPassword 哈希明文密码并写入 users，同时清除 must_change 标记。
//
// 参数：userID — 目标账号；newPass — 明文（≥8 位，由 HashPassword 校验）。
// 返回：HashPassword 或 UpdateUserPassword 的错误。
// 副作用：写 users.password_hash、must_change_password=0。
// 说明：供 ChangePassword / ResetPasswordByAdmin 复用，避免两处相同实现漂移。
func (s *Service) setPassword(userID int64, newPass string) error {
	hash, err := HashPassword(newPass)
	if err != nil {
		return err
	}
	return s.store.UpdateUserPassword(userID, hash, true)
}

// ChangePassword 当前用户修改自己的登录密码并清除 must_change 标记。
//
// 参数：newPass — 明文新密码（≥8 位）。
// 返回：HashPassword 或 UpdateUserPassword 的错误。
// 副作用：写 users.password_hash、must_change_password=0。
func (s *Service) ChangePassword(userID int64, newPass string) error {
	return s.setPassword(userID, newPass)
}

// ResetPasswordByAdmin 管理员重置指定用户密码并清除 must_change 标记。
//
// 参数：targetID — 被重置账号；newPass — 新明文密码。
// 返回：HashPassword 或 UpdateUserPassword 的错误。
// 副作用：写 users 表；踢线由 api 层 sessionmgr 编排。
func (s *Service) ResetPasswordByAdmin(targetID int64, newPass string) error {
	return s.setPassword(targetID, newPass)
}
