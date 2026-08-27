// 一次性工具：确保公司测试账号存在于 home SQLite（不依赖 API 已启动）。
// 用法（仓库根目录）：go run ./scripts/ensure_company_test_user
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/persist"
	"haovpn/internal/security"
)

const (
	testUser = "company_test"
	testPass = "CompanyTest@2026"
	testIP   = "10.88.0.88"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fatal("%v", err)
	}
	dataDir := filepath.Join(root, "home", "data")
	dbPath := filepath.Join(dataDir, "haovpn.db")
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
		dataDir = filepath.Dir(dbPath)
	}

	store, err := persist.Open(dbPath)
	if err != nil {
		fatal("打开数据库: %v", err)
	}
	defer store.Close()

	keyEnc, err := security.LoadOrCreateDataKey(config.DatabaseSection{}, dataDir)
	if err != nil {
		fatal("密钥: %v", err)
	}

	hash, err := auth.HashPassword(testPass)
	if err != nil {
		fatal("哈希: %v", err)
	}

	u, err := store.GetUserByUsername(testUser)
	if err == nil && u != nil {
		if err := store.UpdateUserPassword(u.ID, hash, true); err != nil {
			fatal("更新密码: %v", err)
		}
		if err := store.SetUserEnabled(u.ID, true); err != nil {
			fatal("启用账号: %v", err)
		}
		fmt.Printf("已更新账号 %s id=%d（密码已重置、must_change 已清）\n", testUser, u.ID)
		return
	}

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		fatal("密钥对: %v", err)
	}
	privEnc, err := keyEnc.SealPrivateKey(kp.PrivateKey)
	if err != nil {
		fatal("加密私钥: %v", err)
	}
	id, err := store.CreateVPNAccount(persist.User{
		Username:           testUser,
		PasswordHash:       hash,
		MustChangePassword: false,
		PublicKey:          kp.PublicKey,
		PrivateKeyEnc:      privEnc,
		VPNIP:              testIP,
		AllowedIPs:         []string{"192.168.31.0/24", "10.88.0.0/24"},
		IPMode:             persist.IPModeFixed,
		IPLeaseSec:         86400,
		PolicyVer:          1,
		Enabled:            true,
	})
	if err != nil {
		fatal("创建账号: %v", err)
	}
	fmt.Printf("已创建账号 %s id=%d vpn_ip=%s\n", testUser, id, testIP)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
