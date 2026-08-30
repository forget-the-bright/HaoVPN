package health

import (

	"fmt"

	"os"

	"runtime"

	"time"



	"haovpn/internal/config"
	"haovpn/internal/fileutil"
	"haovpn/internal/logger"

	"haovpn/internal/persist"

)



// Checker 启动前环境与依赖检查。

type Checker struct {

	cfg        *config.ServerConfig

	store      *persist.Store

	configPath string

}



// NewChecker 创建自检器。

func NewChecker(cfg *config.ServerConfig, store *persist.Store, configPath string) *Checker {

	return &Checker{cfg: cfg, store: store, configPath: configPath}

}



// Result 单项检查结果。

type Result struct {

	Name    string `json:"name"`

	OK      bool   `json:"ok"`

	Message string `json:"message"`

}



// RunStartupChecks 执行启动前自检：配置目录、数据库、证书、权限提示。

func (c *Checker) RunStartupChecks() ([]Result, error) {

	var results []Result



	dbDir := dirOf(c.cfg.Database.Path)
	_ = dbDir

	if err := fileutil.EnsureParentDir(c.cfg.Database.Path, 0o755); err != nil {

		results = append(results, Result{"database_dir", false, err.Error()})

		return results, fmt.Errorf("数据库目录不可写: %w", err)

	}

	results = append(results, Result{"database_dir", true, "ok"})



	if c.store != nil {

		if err := c.store.DB().Ping(); err != nil {

			results = append(results, Result{"database_ping", false, err.Error()})

			return results, fmt.Errorf("数据库连接失败: %w", err)

		}

		results = append(results, Result{"database_ping", true, "ok"})

	}



	certOK := fileutil.Exists(c.cfg.Server.TLS.CertFile) && fileutil.Exists(c.cfg.Server.TLS.KeyFile)

	if !certOK && c.cfg.Server.TLS.AutoGenerate {

		results = append(results, Result{"tls_cert", true, "将自动生成自签证书"})

	} else if !certOK {

		results = append(results, Result{"tls_cert", false, "证书文件不存在且 auto_generate=false"})

		return results, fmt.Errorf("TLS 证书不存在: %s", c.cfg.Server.TLS.CertFile)

	} else {

		results = append(results, Result{"tls_cert", true, "ok"})

	}



	warnFilePerm(c.configPath)

	warnFilePerm(c.cfg.Database.Path)

	keyFile := c.cfg.Database.EncryptionKeyFile

	if keyFile == "" {

		keyFile = dbDir + string(os.PathSeparator) + ".haovpn-key"

	}

	warnFilePerm(keyFile)



	if runtime.GOOS != "windows" {

		logger.Info("自检: Linux/macOS 运行 VPN 服务端通常需要 root 或 CAP_NET_ADMIN")

	} else {

		logger.Info("自检: Windows 运行 VPN 需要管理员权限")

	}

	results = append(results, Result{"platform", true, runtime.GOOS + "/" + runtime.GOARCH})



	return results, nil

}



func warnFilePerm(path string) {
	if open, perm := fileutil.CheckWorldReadable(path); open {
		if runtime.GOOS == "windows" {
			logger.Warn("文件 ACL 过宽（Everyone 可读）%s，建议仅 Administrators+SYSTEM（生产环境）", path)
			return
		}
		logger.Warn("文件权限过宽 %s: %o，建议 chmod 600（生产环境）", path, perm)
	}
}



// Status 运行态健康信息（供 /api/v1/health 与 Dashboard）。

type Status struct {

	OK           bool      `json:"ok"`

	UptimeSec    int64     `json:"uptime_sec"`

	OnlinePeers  int       `json:"online_peers"`

	DBOK         bool      `json:"db_ok"`

	TunOK        bool      `json:"tun_ok"`

	NatOK        bool      `json:"nat_ok"`

	RecentErrors []string  `json:"recent_errors,omitempty"`

	StartedAt    time.Time `json:"started_at"`

}



// NewStatus 构造运行态状态。

func NewStatus(started time.Time, onlinePeers int, dbOK, tunOK, natOK bool, recent []string) Status {

	ok := dbOK && tunOK && natOK

	return Status{

		OK:           ok,

		UptimeSec:    int64(time.Since(started).Seconds()),

		OnlinePeers:  onlinePeers,

		DBOK:         dbOK,

		TunOK:        tunOK,

		NatOK:        natOK,

		RecentErrors: recent,

		StartedAt:    started,

	}

}



func dirOf(path string) string {

	for i := len(path) - 1; i >= 0; i-- {

		if path[i] == '/' || path[i] == '\\' {

			return path[:i]

		}

	}

	return "."

}


