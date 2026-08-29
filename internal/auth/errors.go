package auth

import "errors"

// 握手/登录哨兵错误：对外 Error() 为中文展示文案；分类与 fatal 判定用 errors.Is，
// 避免 tunnel/clientapp 靠子串匹配导致文案漂移（如锁定文案与「登录已锁定」不一致）。
//
// 关联：clientapp.IsFatalHandshakeError、tunnel.rejectHandshake 探针签名、probedefense ignore 列表。

var (
	// ErrBadCredentials 用户名不存在或密码错误（统一模糊文案，防账号枚举）。
	ErrBadCredentials = errors.New("用户名或密码错误")

	// ErrAccountDisabled 账号 enabled=0；隧道/Web 共用（Phase D 起 Web 亦走模糊文案时可包装）。
	ErrAccountDisabled = errors.New("账号已禁用")

	// ErrLoginLocked 同一 IP 连续失败达阈值后的锁定（文案须与 fatal 列表一致）。
	ErrLoginLocked = errors.New("登录失败次数过多，请稍后再试")

	// ErrMustChangePassword 须先改密才能连 VPN。
	ErrMustChangePassword = errors.New("须先修改密码后再连接 VPN（请用 Web 管理端或联系管理员）")

	// ErrNoVPN 账号无公钥/私钥，未开通隧道。
	ErrNoVPN = errors.New("账号未开通 VPN（无密钥）")

	// ErrNotAdmin Web 登录拒绝非管理账号（隧道不用）。
	ErrNotAdmin = errors.New("非管理账号，无法登录 Web")

	// ErrPasswordRequired 握手未带密码。
	ErrPasswordRequired = errors.New("请提供密码")

	// ErrUsePasswordLogin 拒绝已废弃的公钥模式。
	ErrUsePasswordLogin = errors.New("请使用账号密码登录")

	// ErrInvalidHandshake 握手请求无效。
	ErrInvalidHandshake = errors.New("无效握手请求")

	// ErrAccountAlreadyOnline 同账号已有在线会话且策略为 reject_second（无法顶替）时返回。
	// 文案固定；clientapp 用 errors.Is / 有限重试，勿改 Error() 文案。
	// 关联：sessionmgr.RegisterVPN、tunnel.classifyHandshakeReject、clientapp.IsAccountAlreadyOnline。
	ErrAccountAlreadyOnline = errors.New("该账号已在其他设备在线")
)
