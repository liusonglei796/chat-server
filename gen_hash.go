//go:build ignore

// 一次性脚本：生成 bcrypt 密码哈希
// 用法: go run gen_hash.go
package main

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	fmt.Println(string(hash))
}
