// Package password 密码哈希：Argon2id（m=65536,t=3,p=4）为主，兼容 bcrypt 历史值透明升级。
// 对齐 02 - 数据库设计.md B 章通用约定（密码哈希唯一口径为 Argon2id）。
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const (
	argonSaltLen = 16
	argonKeyLen  = 32
	argonTime    = 3
	argonMemory  = 64 * 1024 // 65536 KiB
	argonThreads = 4
)

// Hash 使用 Argon2id 生成密码哈希。
func Hash(plain string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("生成盐失败: %w", err)
	}
	key := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding.EncodeToString(key)
	saltEnc := base64.RawStdEncoding.EncodeToString(salt)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads, saltEnc, enc), nil
}

// Verify 校验密码：优先 Argon2id；bcrypt 历史值验证成功后返回 needsUpgrade=true（下次登录透明升级）。
func Verify(plain, stored string) (ok bool, needsUpgrade bool) {
	if strings.HasPrefix(stored, "$argon2id$") {
		return verifyArgon2id(plain, stored), false
	}
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$") {
		err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(plain))
		return err == nil, true
	}
	return false, false
}

func verifyArgon2id(plain, stored string) bool {
	parts := strings.Split(stored, "$")
	// parts: ["", "argon2id", "v=19", "m=65536,t=3,p=4", "salt", "hash"]
	if len(parts) != 6 {
		return false
	}
	var v uint32
	var m, t, p uint32
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(plain), salt, t, m, uint8(p), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
