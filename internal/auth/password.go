package auth

import (
	"errors"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// ValidatePasswordStrength 校验密码强度：≥8 位、≤72 位（bcrypt 有效上限），且须含字母与数字。
//
// 上限 72：bcrypt 只使用前 72 字节，更长密码既误导用户又浪费 CPU；拒绝超长可防 DoS。
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return errors.New("密码至少 8 位")
	}
	if len(password) > 72 {
		return errors.New("密码至多 72 位")
	}
	var hasLetter, hasDigit bool
	for _, c := range password {
		if unicode.IsLetter(c) {
			hasLetter = true
		}
		if unicode.IsDigit(c) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errors.New("密码须同时包含字母与数字")
	}
	return nil
}

// HashPassword 使用 bcrypt（cost=12）生成密码哈希，供入库或改密。
func HashPassword(password string) (string, error) {
	if err := ValidatePasswordStrength(password); err != nil {
		return "", err
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword 校验明文密码是否与 bcrypt 哈希匹配。
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
