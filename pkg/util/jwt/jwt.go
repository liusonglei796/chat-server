package jwt

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret             string
	AccessTokenExpiry  time.Duration // Access Token 有效期
	RefreshTokenExpiry time.Duration // Refresh Token 有效期
}

// 全局配置，由 Init 函数初始化
var jwtConfig *JWTConfig

// Init 初始化 JWT 配置
func Init(secret string, accessExpiryMinutes, refreshExpiryHours int) {
	jwtConfig = &JWTConfig{
		Secret:             secret,
		AccessTokenExpiry:  time.Duration(accessExpiryMinutes) * time.Minute,
		RefreshTokenExpiry: time.Duration(refreshExpiryHours) * time.Hour,
	}
}

// Claims 自定义 JWT 声明
type Claims struct {
	UserID  string `json:"user_id"`
	TokenID string `json:"token_id,omitempty"` // 仅 Refresh Token 使用，用于单点互踢
	jwt.RegisteredClaims
}

// GenerateAccessToken 生成 Access Token (短期，用于接口认证)
func GenerateAccessToken(userID string) (string, error) {
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtConfig.AccessTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "kama_chat",
			Subject:   "access_token",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtConfig.Secret))
}

// GenerateRefreshToken 生成 Refresh Token (长期，用于刷新 Access Token)
// 返回 token 字符串和 tokenID (用于 Redis 存储实现单点互踢)
func GenerateRefreshToken(userID string) (tokenString string, tokenID string, err error) {
	tokenID = uuid.NewString()
	claims := Claims{
		UserID:  userID,
		TokenID: tokenID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(jwtConfig.RefreshTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "kama_chat",
			Subject:   "refresh_token",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err = token.SignedString([]byte(jwtConfig.Secret))
	return
}

// ParseToken 解析并验证 Token
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(jwtConfig.Secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrSignatureInvalid
}
