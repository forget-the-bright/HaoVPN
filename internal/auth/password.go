package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword 使用 bcrypt（cost=12）生成密码哈希，供入库或改密。
//
// 参数：password 长度须 ≥8，否则返回错误。
// 返回：可存入 PasswordHash 的字符串；err 为 bcrypt 或校验失败。
// 副作用：无；纯函数。
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword 校验明文密码是否与 bcrypt 哈希匹配。
//
// 参数：hash 为库中 PasswordHash；password 为本次输入。
// 返回：匹配 true，否则 false；不区分「用户不存在」与「密码错误」（由上层统一文案）。
// 副作用：无；可并发调用。
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
