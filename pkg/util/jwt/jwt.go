// Package jwt 提供 JWT 生成与解析工具
package jwt

import (
	"time" // 时间相关

	"github.com/golang-jwt/jwt/v5" // JWT 库
	"github.com/google/uuid"       // UUID 生成
)

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret             string        // 签名密钥
	AccessTokenExpiry  time.Duration // Access Token 有效期
	RefreshTokenExpiry time.Duration // Refresh Token 有效期
}

// 全局配置，由 Init 函数初始化
var jwtConfig *JWTConfig

// Init 初始化 JWT 配置
func Init(secret string, accessExpiryMinutes, refreshExpiryHours int) {
	jwtConfig = &JWTConfig{ // 构建配置
		Secret:             secret,                                           // 写入密钥
		AccessTokenExpiry:  time.Duration(accessExpiryMinutes) * time.Minute, // 分钟转时长
		RefreshTokenExpiry: time.Duration(refreshExpiryHours) * time.Hour,    // 小时转时长
	} // 赋值全局配置
}

// Claims 自定义 JWT 声明
type Claims struct {
	UserID               string `json:"user_id"`            // 用户 ID
	TokenID              string `json:"token_id,omitempty"` // 仅 Refresh Token 使用，用于单点互踢
	jwt.RegisteredClaims        // 标准声明
}

// GenerateAccessToken 生成 Access Token (短期，用于接口认证)
func GenerateAccessToken(userID string) (string, error) {
	claims := Claims{ // 构造声明
		UserID: userID, // 设置用户 ID
		RegisteredClaims: jwt.RegisteredClaims{ // 填充标准字段
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtConfig.AccessTokenExpiry)), // 过期时间
			IssuedAt:  jwt.NewNumericDate(time.Now()),                                  // 签发时间
			Issuer:    "kama_chat",                                                     // 签发者
			Subject:   "access_token",                                                  // 主题
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims) // 使用 HS256 签名算法
	return token.SignedString([]byte(jwtConfig.Secret))        // 返回签名后的字符串
}

// GenerateRefreshToken 生成 Refresh Token (长期，用于刷新 Access Token)
// 返回 token 字符串和 tokenID (用于 Redis 存储实现单点互踢)
func GenerateRefreshToken(userID string) (tokenString string, tokenID string, err error) {
	tokenID = uuid.NewString() // 生成 tokenID
	claims := Claims{          // 构造声明
		UserID:  userID,  // 设置用户 ID
		TokenID: tokenID, // 写入 tokenID
		RegisteredClaims: jwt.RegisteredClaims{ // 填充标准字段
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtConfig.RefreshTokenExpiry)), // 过期时间
			IssuedAt:  jwt.NewNumericDate(time.Now()),                                   // 签发时间
			Issuer:    "kama_chat",                                                      // 签发者
			Subject:   "refresh_token",                                                  // 主题
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)      // 使用 HS256 签名算法
	tokenString, err = token.SignedString([]byte(jwtConfig.Secret)) // 返回签名后的字符串
	return                                                          // 返回 tokenString、tokenID、err
}

// ParseToken 解析并验证 Token
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) { // 解析并校验
		return []byte(jwtConfig.Secret), nil // 提供签名密钥
	})
	if err != nil { // 解析失败
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid { // 类型断言并校验合法性
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid // 签名无效
}
