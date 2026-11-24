package security

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword 对明文密码进行 Bcrypt 哈希计算
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// VerifyPassword 校验明文密码与哈希值是否匹配
func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
