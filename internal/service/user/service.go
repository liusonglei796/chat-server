package user

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/dto/event"
	"kama_chat_server/internal/dto/request/auth"
	userreq "kama_chat_server/internal/dto/request/user"
	userrsp "kama_chat_server/internal/dto/respond/user"
	cacheutil "kama_chat_server/internal/infrastructure/cache"
	"kama_chat_server/internal/infrastructure/jwt"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
)

// UserService 用户业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖
type UserService struct {
	uow         repository.UnitOfWork
	cache       repository.AsyncCacheService
	cacheHelper *cacheutil.Helper // 缓存辅助工具（带 singleflight）
	outboxRepo  repository.OutboxRepository
}

// NewUserService 构造函数，注入所有依赖
func NewUserService(uow repository.UnitOfWork, cacheService repository.AsyncCacheService, outboxRepo repository.OutboxRepository) *UserService {
	return &UserService{
		uow:         uow,
		cache:       cacheService,
		cacheHelper: cacheutil.NewHelper(cacheService),
		outboxRepo:  outboxRepo,
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
// 包含：状态检查 → 生成双 Token → SSO 存储 → 构建响应
func (u *UserService) buildLoginResponse(ctx context.Context, user *model.UserInfo, clientIP string) (*userrsp.LoginRespond, error) {
	// 1. 检查用户状态
	if user.Status == user_status.DISABLE {
		return nil, errorx.New(errorx.CodeForbidden, "该账号已被禁用，请联系管理员")
	}

	// 2. 生成双 Token (传入 isAdmin 用于 JWT Claims)
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

	// 3. SSO: 将 Access Token 存入 Redis，实现单点登录（覆盖旧 token）
	ssoTokenKey := constants.CacheKeySSOToken + user.Uuid
	ssoTTL := time.Duration(constants.SSO_TOKEN_EXPIRY_HOURS) * time.Hour

	// 存储 token（SSO: 新登录会覆盖旧 token）
	if err := u.cache.Set(ctx, ssoTokenKey, accessToken, ssoTTL); err != nil {
		zap.L().Error("存储 SSO Token 到 Redis 失败", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 4. 将 Refresh Token ID 存入缓存，实现单点互踢
	redisKey := constants.CacheKeyUserToken + user.Uuid
	if err := u.cache.Set(ctx, redisKey, tokenID, time.Duration(constants.REFRESH_TOKEN_EXPIRY_HOURS)*time.Hour); err != nil {
		zap.L().Error("存储 Token ID 到缓存失败", zap.Error(err))
	}

	// 5. 构建响应
	loginRsp := &userrsp.LoginRespond{
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
// SSO: 登录时会将 token 存入 Redis，实现单点登录
func (u *UserService) Login(ctx context.Context, loginReq auth.LoginRequest, clientIP string) (*userrsp.LoginRespond, error) {
	user, err := u.uow.UserRepo().FindByNickname(ctx, loginReq.Username)
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
func (u *UserService) Register(ctx context.Context, registerReq auth.RegisterRequest, clientIP string) (*userrsp.LoginRespond, error) {
	// 检查用户名是否已存在
	existing, err := u.uow.UserRepo().FindByNickname(ctx, registerReq.Username)
	if err == nil && existing != nil {
		return nil, errorx.New(errorx.CodeUserExist, "用户名已被占用")
	}

	// 生成 UUID
	uuid := "U" + fmt.Sprintf("%d", time.Now().UnixMilli())

	// 创建用户
	user := &model.UserInfo{
		Uuid:        uuid,
		Nickname:    registerReq.Username,
		Telephone:   "",
		Password:    "", // Will be set via RawPassword + BeforeSave
		RawPassword: registerReq.Password,
		IsAdmin:     0,
		Status:      0,
	}

	if err := u.uow.UserRepo().CreateUser(ctx, user); err != nil {
		zap.L().Error("创建用户失败", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	return u.buildLoginResponse(ctx, user, clientIP)
}

// Logout 用户登出
// 从 Redis 删除 SSO token
func (u *UserService) Logout(ctx context.Context, userId string) error {
	ssoTokenKey := constants.CacheKeySSOToken + userId

	// 删除 SSO token
	if err := u.cache.Delete(ctx, ssoTokenKey); err != nil {
		zap.L().Error("删除 SSO Token 失败", zap.Error(err))
		return errorx.ErrServerBusy
	}

	return nil
}

// KickUser 管理员踢人下线
// 从 Redis 删除指定用户的 SSO token
func (u *UserService) KickUser(ctx context.Context, userId string) error {
	// 检查用户是否存在
	_, err := u.uow.UserRepo().FindByUuid(ctx, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	ssoTokenKey := constants.CacheKeySSOToken + userId

	// 删除 SSO token
	if err := u.cache.Delete(ctx, ssoTokenKey); err != nil {
		zap.L().Error("删除 SSO Token 失败", zap.Error(err))
		return errorx.ErrServerBusy
	}

	return nil
}

// UpdateUserInfo 修改用户信息
// UpdateUserInfo 修改用户信息 (userId 从 JWT 获取，只能改自己)
// 使用指针类型区分"未传字段"(nil=不更新)和"清空字段"(""=置空)
// 警告问题修复：使用数据库事务确保用户信息和会话信息的一致性
func (u *UserService) UpdateUserInfo(ctx context.Context, userId string, updateReq userreq.UpdateUserInfoRequest) error {
	user, err := u.uow.UserRepo().FindByUuid(ctx, userId)
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

	// 警告问题修复：使用事务管理确保数据一致性
	// 事务内的操作：1.更新用户信息 2.更新会话冗余字段
	// 任一操作失败都会回滚，保证数据一致性
	if err := u.uow.WithTx(func(tx repository.UnitOfWork) error {
		// 1. 在事务内更新用户信息
		if err := tx.UserRepo().UpdateUserInfo(ctx, user); err != nil {
			return err
		}

		// 2. 事务内写 outbox 事件，message_service 消费后更新 session 冗余字段
		nick := updateReq.Nickname
		av := updateReq.Avatar
		if nick != nil || av != nil {
			payload, _ := json.Marshal(event.UserUpdatedEvent{UserId: userId, Nickname: nick, Avatar: av})
			o := model.Outbox{
				Uuid:      fmt.Sprintf("O%s", snowflake.GenerateIDString()),
				EventType: event.EventUserUpdated,
				Payload:   string(payload),
				Status:    0,
				CreatedAt: time.Now(),
			}
			if err := tx.OutboxRepo().Create(ctx, &o); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		zap.L().Error("更新用户信息事务失败", zap.String("userId", userId), zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 异步清理缓存（含空值标记，防止空值缓存残留导致数据不一致）
	u.cache.SubmitTask(func() {
		// 清理完整用户信息缓存 + 空值标记
		if err := u.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyUserInfo+userId); err != nil {
			zap.L().Error("清理用户信息缓存失败", zap.Error(err))
		}
		// 清理公开用户信息缓存 + 空值标记
		if err := u.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyUserPublicInfo+userId); err != nil {
			zap.L().Error("清理公开用户信息缓存失败", zap.Error(err))
		}
	})

	return nil
}

// GetUserInfo 获取用户完整信息（仅限自己调用）
// 使用 Cache-Aside 模式 + singleflight 防止缓存击穿
func (u *UserService) GetUserInfo(ctx context.Context, requesterId, targetId string) (*userrsp.GetUserInfoRespond, error) {
	// 权限校验: 只能查看自己的完整信息
	if requesterId != targetId {
		return nil, errorx.New(errorx.CodeForbidden, "无权查看他人详细信息")
	}

	key := constants.CacheKeyUserInfo + targetId
	var rsp userrsp.GetUserInfoRespond

	err := u.cacheHelper.GetOrLoad(
		ctx,
		key,
		func(loaderCtx context.Context) (interface{}, error) {
			user, err := u.uow.UserRepo().FindByUuid(loaderCtx, targetId)
			if err != nil {
				if errorx.IsNotFound(err) {
					return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
				}
				return nil, err
			}
			return userrsp.GetUserInfoRespond{
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
		cacheutil.RandomizedTTL(time.Hour), // 数据 TTL (带抖动防雪崩)
		5*time.Minute,                      // 空值 TTL (防穿透)
		&rsp,
	)
	if err != nil {
		return nil, err
	}
	return &rsp, nil
}

// GetPublicUserInfo 获取用户公开信息（查看他人）
// 使用 Cache-Aside 模式 + singleflight 防止缓存击穿
func (u *UserService) GetPublicUserInfo(ctx context.Context, targetId string) (*userrsp.PublicUserInfoRespond, error) {
	key := constants.CacheKeyUserPublicInfo + targetId
	var rsp userrsp.PublicUserInfoRespond

	err := u.cacheHelper.GetOrLoad(
		ctx,
		key,
		func(loaderCtx context.Context) (interface{}, error) {
			user, err := u.uow.UserRepo().FindByUuid(loaderCtx, targetId)
			if err != nil {
				if errorx.IsNotFound(err) {
					return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
				}
				return nil, err
			}
			return userrsp.PublicUserInfoRespond{
				Uuid:      user.Uuid,
				Nickname:  user.Nickname,
				Avatar:    user.Avatar,
				Gender:    user.Gender,
				Birthday:  user.Birthday,
				Signature: user.Signature,
			}, nil
		},
		cacheutil.RandomizedTTL(30*time.Minute), // 数据 TTL (带抖动防雪崩)
		5*time.Minute, // 空值 TTL (防穿透)
		&rsp,
	)
	if err != nil {
		return nil, err
	}
	return &rsp, nil
}

// BatchGetPublicUserInfo 批量获取公开用户信息（昵称/头像/性别等）
// 供跨服务列表页一次拉取，避免 N+1 调用
func (u *UserService) BatchGetPublicUserInfo(ctx context.Context, userIds []string) ([]userrsp.PublicUserInfoRespond, error) {
	if len(userIds) == 0 {
		return []userrsp.PublicUserInfoRespond{}, nil
	}
	userList, err := u.uow.UserRepo().FindByUuids(ctx, userIds)
	if err != nil {
		zap.L().Error("batch find users error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	rsp := make([]userrsp.PublicUserInfoRespond, 0, len(userList))
	for _, usr := range userList {
		rsp = append(rsp, userrsp.PublicUserInfoRespond{
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
// 供跨服务在写操作前置校验目标用户状态，避免读整个用户表
func (u *UserService) GetUserStatus(ctx context.Context, userId string) (int8, error) {
	user, err := u.uow.UserRepo().FindByUuid(ctx, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return 0, errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		return 0, errorx.ErrServerBusy
	}
	return user.Status, nil
}
