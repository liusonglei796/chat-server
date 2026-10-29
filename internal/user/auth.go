package user

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/idempotency"
	"kama_chat_server/internal/infra/jwt"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/idempotent"
)

// buildLoginResponse 构建登录的公共响应
func (u *UserService) buildLoginResponse(ctx context.Context, user *UserInfo, clientIP string) (*LoginRespond, error) {
	if user.Status == user_status.DISABLE {
		return nil, errorx.New(errorx.CodeForbidden, "该账号已被禁用，请联系管理员")
	}

	accessToken, err := jwt.GenerateAccessToken(user.Uuid, user.IsAdmin == 1)
	if err != nil {
		zap.L().Error("生成 Access Token 失败", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	refreshToken, tokenID, err := jwt.GenerateRefreshToken(user.Uuid)
	if err != nil {
		zap.L().Error("生成 Refresh Token 失败", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	if u.cache != nil {
		ssoTokenKey := constants.CacheKeySSOToken + user.Uuid
		ssoTTL := time.Duration(constants.SSO_TOKEN_EXPIRY_HOURS) * time.Hour
		if err := u.cache.Set(ctx, ssoTokenKey, accessToken, ssoTTL); err != nil {
			zap.L().Error("存储 SSO Token 到 Redis 失败", zap.Error(err))
			return nil, errorx.ErrServerBusy
		}

		redisKey := constants.CacheKeyUserToken + user.Uuid
		if err := u.cache.Set(ctx, redisKey, tokenID, time.Duration(constants.REFRESH_TOKEN_EXPIRY_HOURS)*time.Hour); err != nil {
			zap.L().Error("存储 Token ID 到缓存失败", zap.Error(err))
		}
	}

	loginRsp := &LoginRespond{
		Uuid:         user.Uuid,
		Telephone:    user.Telephone,
		Nickname:     user.Nickname,
		Email:        user.Email,
		Avatar:       user.Avatar,
		Gender:       user.Gender,
		Birthday:     user.Birthday,
		Signature:    user.Signature,
		IsAdmin:      user.IsAdmin,
		Status:       user.Status,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
	year, month, day := user.CreatedAt.Date()
	loginRsp.CreatedAt = fmt.Sprintf("%d.%d.%d", year, month, day)

	return loginRsp, nil
}

// Login 登录（密码登录）
func (u *UserService) Login(ctx context.Context, loginReq LoginRequest, clientIP string) (*LoginRespond, error) {
	user, err := u.store.FindByNickname(ctx, loginReq.Username)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if !user.CheckPassword(loginReq.Password) {
		return nil, errorx.New(errorx.CodeInvalidPassword, "密码不正确，请重试")
	}

	return u.buildLoginResponse(ctx, user, clientIP)
}

// Register 用户注册
func (u *UserService) Register(ctx context.Context, registerReq RegisterRequest, clientIP string) (*RegisterRespond, error) {
	action := func(c context.Context) (*RegisterRespond, error) {
		existing, err := u.store.FindByNickname(c, registerReq.Username)
		if err == nil && existing != nil {
			return nil, errorx.New(errorx.CodeUserExist, "用户名已被占用")
		}

		uuid := "U" + fmt.Sprintf("%d", time.Now().UnixMilli())

		user := &UserInfo{
			Uuid:        uuid,
			Nickname:    registerReq.Username,
			Telephone:   "",
			Password:    "",
			RawPassword: registerReq.Password,
			IsAdmin:     0,
			Status:      0,
		}

		if err := u.store.CreateUser(c, user); err != nil {
			if dberr.IsDuplicateKey(err) {
				return nil, errorx.New(errorx.CodeUserExist, "用户名已被占用")
			}
			zap.L().Error("创建用户失败", zap.Error(err))
			return nil, errorx.ErrServerBusy
		}

		return &RegisterRespond{
			Uuid:      user.Uuid,
			Nickname:  user.Nickname,
			Telephone: user.Telephone,
			Avatar:    user.Avatar,
			Email:     user.Email,
			Gender:    user.Gender,
			Birthday:  user.Birthday,
			Signature: user.Signature,
			CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
			IsAdmin:   user.IsAdmin,
			Status:    user.Status,
		}, nil
	}

	if registerReq.ClientToken != "" && u.idempStore != nil {
		rsp, isReplay, err := idempotency.ExecuteJSON(ctx, u.idempStore, registerReq.Username, "Register", registerReq.ClientToken, action)
		if isReplay {
			zap.L().Info("命中用户注册持久化幂等表回放，直接返回已创建用户实体", zap.String("userId", rsp.Uuid), zap.String("username", registerReq.Username))
		}
		return rsp, err
	} else if registerReq.ClientToken != "" && u.guard != nil {
		key := fmt.Sprintf("auth:reg:%s:%s", registerReq.Username, registerReq.ClientToken)
		rsp, isReplay, err := idempotent.ExecuteJSON(ctx, u.guard, key, 5*time.Second, 24*time.Hour, action)
		if isReplay {
			zap.L().Info("命中用户注册幂等回放，直接返回已创建用户实体", zap.String("userId", rsp.Uuid), zap.String("username", registerReq.Username))
		}
		return rsp, err
	}

	return action(ctx)
}

// Logout 用户登出
func (u *UserService) Logout(ctx context.Context, userId string) error {
	if u.cache != nil {
		ssoTokenKey := constants.CacheKeySSOToken + userId
		if err := u.cache.Delete(ctx, ssoTokenKey); err != nil {
			zap.L().Error("删除 SSO Token 失败", zap.Error(err))
			return errorx.ErrServerBusy
		}
	}
	return nil
}

// KickUser 管理员踢人下线
func (u *UserService) KickUser(ctx context.Context, userId string) error {
	_, err := u.store.FindByUuid(ctx, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	if u.cache != nil {
		ssoTokenKey := constants.CacheKeySSOToken + userId
		if err := u.cache.Delete(ctx, ssoTokenKey); err != nil {
			zap.L().Error("删除 SSO Token 失败", zap.Error(err))
			return errorx.ErrServerBusy
		}
	}

	return nil
}

// ==================== AuthService ====================

// AuthService 认证服务实现
type AuthService struct {
	cache     cacheinfra.CacheService
	userStore Store
}

// NewAuthService 创建认证服务实例
func NewAuthService(cache cacheinfra.CacheService, userStore Store) *AuthService {
	return &AuthService{
		cache:     cache,
		userStore: userStore,
	}
}

// ValidateTokenID 验证用户的 Token ID 是否有效
func (s *AuthService) ValidateTokenID(ctx context.Context, userID, tokenID string) (bool, error) {
	if s.cache == nil {
		return true, nil
	}
	redisKey := constants.CacheKeyUserToken + userID
	validTokenID, err := s.cache.Get(ctx, redisKey)
	if err != nil {
		return false, err
	}
	if validTokenID == "" {
		return false, nil
	}
	return tokenID == validTokenID, nil
}

// GetUserIsAdmin 获取用户是否为管理员
func (s *AuthService) GetUserIsAdmin(ctx context.Context, userID string) (bool, error) {
	user, err := s.userStore.FindByUuid(ctx, userID)
	if err != nil {
		if errorx.IsNotFound(err) {
			return false, errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		return false, err
	}
	return user.IsAdmin == 1, nil
}
