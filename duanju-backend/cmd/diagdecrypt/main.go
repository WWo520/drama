// 诊断/迁移工具：解密验证（2 参数）或跨密钥重加密（3 参数）model_keys.key_encrypted。
// 用法：
//
//	diagdecrypt <secret> name=enc...           # 解密验证
//	diagdecrypt <oldSecret> <newSecret> name=enc... # 重加密（输出新密文）
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/duanju/backend/internal/pkg/cryptox"
)

func main() {
	args := os.Args[1:]
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: diagdecrypt <secret> [newSecret] name=enc...")
		os.Exit(1)
	}
	oldKey := cryptox.DeriveKey(args[0])
	var newKey []byte
	pairs := args[1:]
	if len(args) >= 3 && !strings.Contains(args[1], "=") {
		newKey = cryptox.DeriveKey(args[1])
		pairs = args[2:]
	}
	for _, arg := range pairs {
		i := strings.Index(arg, "=")
		if i < 0 {
			continue
		}
		name, enc := arg[:i], arg[i+1:]
		plain, err := cryptox.Decrypt(enc, oldKey)
		if err != nil {
			fmt.Printf("%s: DECRYPT_FAIL (%v)\n", name, err)
			continue
		}
		if os.Getenv("DIAG_PLAIN") == "1" {
			fmt.Printf("%s\t%s\n", name, plain)
			continue
		}
		if newKey == nil {
			head := plain
			if len(head) > 6 {
				head = head[:6]
			}
			fmt.Printf("%s: OK (len=%d head=%s***)\n", name, len(plain), head)
			continue
		}
		re, err := cryptox.Encrypt(plain, newKey)
		if err != nil {
			fmt.Printf("%s: ENCRYPT_FAIL (%v)\n", name, err)
			continue
		}
		fmt.Printf("%s: REENC OK, plain_len=%d\n%s\n", name, len(plain), re)
	}
}
