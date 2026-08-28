package vpnaccount

// SetAccountEnabled 启用或禁用账号；禁用时踢线使隧道立即失效。
//
// 参数：id — users.id；enabled — true 启用 / false 禁用。
// 返回：SetUserEnabled 的错误。
// 副作用：写 users.enabled；enabled=false 时调用 OnKickUser。
func (s *Service) SetAccountEnabled(id int64, enabled bool) error {
	if err := s.Store.SetUserEnabled(id, enabled); err != nil {
		return err
	}
	if !enabled && s.OnKickUser != nil {
		s.OnKickUser(id)
	}
	return nil
}
