package user

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/jwt"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/idempotent"
)

// UserService 用户业务逻辑实现
type UserService struct {
	uow         UoW
	cache       cacheinfra.AsyncCacheService
	cacheHelper *cacheinfra.Helper
	guard       idempotent.Guard
}

// NewUserService 构造函数，注入所有依赖
func NewUserService(uow UoW, cacheService cacheinfra.AsyncCacheService, guard ...idempotent.Guard) *UserService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	var helper *cacheinfra.Helper
	if cacheService != nil {
		helper = cacheinfra.NewHelper(cacheService)
	}
	return &UserService{
		uow:         uow,
		cache:       cacheService,
		cacheHelper: helper,
		guard:       g,
	}
}

// checkTelephoneValid 检验电话是否有效
func (u *UserService) checkTelephoneValid(telephone string) bool {
	pattern := `^1([38][0-9]|14[579]|5[^4]|16[6]|7[1-35-8]|9[189])\d{8}$`
	match, err := regexp.MatchString(pattern, telephone)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
	return match
}

// checkEmailValid 校验邮箱是否有效
func (u *UserService) checkEmailValid(email string) bool {
	pattern := `^[^\s@]+@[^\s@]+\.[^\s@]+$`
	match, err := regexp.MatchString(pattern, email)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
	return match
}

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
	user, err := u.uow.UserStore().FindByNickname(ctx, loginReq.Username)
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
		existing, err := u.uow.UserStore().FindByNickname(c, registerReq.Username)
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

		if err := u.uow.UserStore().CreateUser(c, user); err != nil {
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

	if registerReq.ClientToken != "" && u.guard != nil {
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
	_, err := u.uow.UserStore().FindByUuid(ctx, userId)
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

// UpdateUserInfo 修改用户信息
func (u *UserService) UpdateUserInfo(ctx context.Context, userId string, updateReq UpdateUserInfoRequest) error {
	user, err := u.uow.UserStore().FindByUuid(ctx, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if updateReq.Email != nil {
		user.Email = *updateReq.Email
	}
	if updateReq.Nickname != nil {
		user.Nickname = *updateReq.Nickname
	}
	if updateReq.Birthday != nil {
		user.Birthday = *updateReq.Birthday
	}
	if updateReq.Signature != nil {
		user.Signature = *updateReq.Signature
	}
	if updateReq.Avatar != nil {
		user.Avatar = *updateReq.Avatar
	}

	if err := WithTx(u.uow, func(tx UoW) error {
		return tx.UserStore().UpdateUserInfo(ctx, user)
	}); err != nil {
		zap.L().Error("更新用户信息事务失败", zap.String("userId", userId), zap.Error(err))
		return errorx.ErrServerBusy
	}

	if u.cache != nil {
		u.cache.SubmitTask(func() {
			if u.cacheHelper != nil {
				if err := u.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyUserInfo+userId); err != nil {
					zap.L().Error("清理用户信息缓存失败", zap.Error(err))
				}
				if err := u.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyUserPublicInfo+userId); err != nil {
					zap.L().Error("清理公开用户信息缓存失败", zap.Error(err))
				}
			}
		})
	}

	return nil
}

// GetUserInfo 获取用户完整信息（仅限自己调用）
func (u *UserService) GetUserInfo(ctx context.Context, requesterId, targetId string) (*GetUserInfoRespond, error) {
	if requesterId != targetId {
		return nil, errorx.New(errorx.CodeForbidden, "无权查看他人详细信息")
	}

	if u.cacheHelper == nil {
		user, err := u.uow.UserStore().FindByUuid(ctx, targetId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
			}
			return nil, err
		}
		return &GetUserInfoRespond{
			Uuid:      user.Uuid,
			Telephone: user.Telephone,
			Nickname:  user.Nickname,
			Avatar:    user.Avatar,
			Birthday:  user.Birthday,
			Email:     user.Email,
			Gender:    user.Gender,
			Signature: user.Signature,
			CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
			IsAdmin:   user.IsAdmin,
			Status:    user.Status,
		}, nil
	}

	key := constants.CacheKeyUserInfo + targetId
	var rsp GetUserInfoRespond

	err := u.cacheHelper.GetOrLoad(
		ctx,
		key,
		func(loaderCtx context.Context) (interface{}, error) {
			user, err := u.uow.UserStore().FindByUuid(loaderCtx, targetId)
			if err != nil {
				if errorx.IsNotFound(err) {
					return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
				}
				return nil, err
			}
			return GetUserInfoRespond{
				Uuid:      user.Uuid,
				Telephone: user.Telephone,
				Nickname:  user.Nickname,
				Avatar:    user.Avatar,
				Birthday:  user.Birthday,
				Email:     user.Email,
				Gender:    user.Gender,
				Signature: user.Signature,
				CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
				IsAdmin:   user.IsAdmin,
				Status:    user.Status,
			}, nil
		},
		cacheinfra.RandomizedTTL(time.Hour),
		5*time.Minute,
		&rsp,
	)
	if err != nil {
		return nil, err
	}
	return &rsp, nil
}

// GetPublicUserInfo 获取用户公开信息（查看他人）
func (u *UserService) GetPublicUserInfo(ctx context.Context, targetId string) (*PublicUserInfoRespond, error) {
	if u.cacheHelper == nil {
		user, err := u.uow.UserStore().FindByUuid(ctx, targetId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
			}
			return nil, err
		}
		return &PublicUserInfoRespond{
			Uuid:      user.Uuid,
			Nickname:  user.Nickname,
			Avatar:    user.Avatar,
			Gender:    user.Gender,
			Birthday:  user.Birthday,
			Signature: user.Signature,
		}, nil
	}

	key := constants.CacheKeyUserPublicInfo + targetId
	var rsp PublicUserInfoRespond

	err := u.cacheHelper.GetOrLoad(
		ctx,
		key,
		func(loaderCtx context.Context) (interface{}, error) {
			user, err := u.uow.UserStore().FindByUuid(loaderCtx, targetId)
			if err != nil {
				if errorx.IsNotFound(err) {
					return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
				}
				return nil, err
			}
			return PublicUserInfoRespond{
				Uuid:      user.Uuid,
				Nickname:  user.Nickname,
				Avatar:    user.Avatar,
				Gender:    user.Gender,
				Birthday:  user.Birthday,
				Signature: user.Signature,
			}, nil
		},
		cacheinfra.RandomizedTTL(30*time.Minute),
		5*time.Minute,
		&rsp,
	)
	if err != nil {
		return nil, err
	}
	return &rsp, nil
}

// BatchGetPublicUserInfo 批量获取公开用户信息
func (u *UserService) BatchGetPublicUserInfo(ctx context.Context, userIds []string) ([]PublicUserInfoRespond, error) {
	if len(userIds) == 0 {
		return []PublicUserInfoRespond{}, nil
	}
	userList, err := u.uow.UserStore().FindByUuids(ctx, userIds)
	if err != nil {
		zap.L().Error("batch find users error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	rsp := make([]PublicUserInfoRespond, 0, len(userList))
	for _, usr := range userList {
		rsp = append(rsp, PublicUserInfoRespond{
			Uuid:      usr.Uuid,
			Nickname:  usr.Nickname,
			Avatar:    usr.Avatar,
			Gender:    usr.Gender,
			Birthday:  usr.Birthday,
			Signature: usr.Signature,
		})
	}
	return rsp, nil
}

// GetUserStatus 获取用户账号状态（正常/禁用）
func (u *UserService) GetUserStatus(ctx context.Context, userId string) (int8, error) {
	user, err := u.uow.UserStore().FindByUuid(ctx, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return 0, errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		return 0, errorx.ErrServerBusy
	}
	return user.Status, nil
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
