package brand

const (
	// Name 产品显示名。
	Name = "HaoVPN"

	// BinServer / BinClient / BinClientGUI 构建产物基名（Windows 加 .exe）。
	BinServer     = "haovpn-server"
	BinClient     = "haovpn-client"
	BinClientGUI  = "haovpn-client-gui"

	// DefaultTunName 默认 TUN 网卡名。
	DefaultTunName = "haovpn0"

	// DefaultDBFile 默认 SQLite 文件名。
	DefaultDBFile = "haovpn.db"

	// DataKeyFile 字段加密密钥默认文件名（相对 database.path 父目录）。
	DataKeyFile = ".haovpn-key"

	// WintunPool Wintun 适配器池名。
	WintunPool = "HaoVPN"

	// WinNATName Windows New-NetNat 规则名。
	WinNATName = "HaoVPNNat"

	// GUIAppID Fyne 应用 ID。
	GUIAppID = "com.haovpn.client"

	// WinServiceName Windows 服务内部名。
	WinServiceName = "HaoVPNClient"

	// WinServiceDisplay Windows 服务显示名。
	WinServiceDisplay = "HaoVPN Client"

	// CredDirName ProgramData 下凭据目录名。
	CredDirName = "HaoVPN"

	// EnvUser / EnvPassword 客户端环境变量名。
	EnvUser     = "HAOVPN_USER"
	EnvPassword = "HAOVPN_PASSWORD"
)
