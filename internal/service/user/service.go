package user

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/domain/repository"
	"kama_chat_server/internal/dto/request/auth"
	userreq "kama_chat_server/internal/dto/request/user"
	userrsp "kama_chat_server/internal/dto/respond/user"
	cacheutil "kama_chat_server/internal/infrastructure/cache"
	"kama_chat_server/internal/infrastructure/jwt"
	"kama_chat_server/internal/infrastructure/sms"
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
	smsService  *sms.AliyunSmsService
}

// NewUserService 构造函数，注入所有依赖
func NewUserService(uow repository.UnitOfWork, cacheService repository.AsyncCacheService, smsService *sms.AliyunSmsService) *UserService {
	return &UserService{
		uow:         uow,
		cache:       cacheService,
		cacheHelper: cacheutil.NewHelper(cacheService),
		smsService:  smsService,
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

// buildLoginResponse 构建登录/短信登录的公共响应
// 包含：状态检查 → 踢旧设备 → 生成双 Token → 存 Redis → 构建响应
func (u *UserService) buildLoginResponse(ctx context.Context, user *model.UserInfo) (*userrsp.LoginRespond, error) {
	// 1. 检查用户状态
	if user.Status == user_status.DISABLE {
		return nil, errorx.New(errorx.CodeForbidden, "该账号已被禁用，请联系管理员")
	}

	// 2. 踢掉旧设备（如果在线）- 已移除

	// 3. 生成双 Token (传入 isAdmin 用于 JWT Claims)
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

	// 4. 将 Refresh Token ID 存入缓存，实现单点互踢
	redisKey := constants.CacheKeyUserToken + user.Uuid
	if err := u.cache.Set(ctx, redisKey, tokenID, time.Duration(constants.REFRESH_TOKEN_EXPIRY_HOURS)*time.Hour); err != nil {
		zap.L().Error("存储 Token ID 到缓存失败", zap.Error(err))
		// 不阻塞登录流程，仅记录日志
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

// Login 登录
func (u *UserService) Login(ctx context.Context, loginReq auth.LoginRequest) (*userrsp.LoginRespond, error) {
	user, err := u.uow.UserRepo().FindByTelephone(ctx, loginReq.Telephone)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在，请注册")
		}
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if !user.CheckPassword(loginReq.Password) {
		return nil, errorx.New(errorx.CodeInvalidPassword, "密码不正确，请重试")
	}

	return u.buildLoginResponse(ctx, user)
}

// SmsLogin 验证码登录
func (u *UserService) SmsLogin(ctx context.Context, req auth.SmsLoginRequest) (*userrsp.LoginRespond, error) {
	user, err := u.uow.UserRepo().FindByTelephone(ctx, req.Telephone)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在，请注册")
		}
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 校验短信验证码
	key := constants.CacheKeyAuthCode + req.Telephone
	code, err := u.cache.Get(context.Background(), key)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if code != req.SmsCode {
		return nil, errorx.New(errorx.CodeInvalidParam, "验证码不正确，请重试")
	}
	if err := u.cache.Delete(context.Background(), key); err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	return u.buildLoginResponse(ctx, user)
}

// SendSmsCode 发送短信验证码 - 验证码登录
func (u *UserService) SendSmsCode(ctx context.Context, telephone string) error {
	return u.smsService.SendVerificationCode(ctx, telephone)
}

// checkTelephoneExist 检查手机号是否存在
func (u *UserService) checkTelephoneExist(ctx context.Context, telephone string) error {
	_, err := u.uow.UserRepo().FindByTelephone(ctx, telephone)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			zap.L().Info("该电话不存在，可以注册")
			return nil
		}
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	zap.L().Info("该电话已经存在，注册失败")
	return errorx.New(errorx.CodeUserExist, "该电话已经存在，注册失败")
}

// Register 注册
func (u *UserService) Register(ctx context.Context, registerReq auth.RegisterRequest) (*userrsp.RegisterRespond, error) {
	key := constants.CacheKeyAuthCode + registerReq.Telephone
	code, err := u.cache.Get(context.Background(), key)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if code != registerReq.SmsCode {
		return nil, errorx.New(errorx.CodeInvalidParam, "验证码不正确，请重试")
	}
	if err := u.cache.Delete(context.Background(), key); err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 判断电话是否已经被注册过了
	if err := u.checkTelephoneExist(ctx, registerReq.Telephone); err != nil {
		return nil, err
	}

	var newUser model.UserInfo
	newUser.Uuid = "U" + snowflake.GenerateIDString()
	newUser.Telephone = registerReq.Telephone
	newUser.RawPassword = registerReq.Password
	newUser.Nickname = registerReq.Nickname
	newUser.Avatar = "https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png"
	newUser.CreatedAt = time.Now()
	newUser.IsAdmin = 0 // 新注册用户默认非管理员
	newUser.Status = user_status.NORMAL

	err = u.uow.UserRepo().CreateUser(ctx, &newUser)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	registerRsp := &userrsp.RegisterRespond{
		Uuid:      newUser.Uuid,
		Telephone: newUser.Telephone,
		Nickname:  newUser.Nickname,
		Email:     newUser.Email,
		Avatar:    newUser.Avatar,
		Gender:    newUser.Gender,
		Birthday:  newUser.Birthday,
		Signature: newUser.Signature,
		IsAdmin:   newUser.IsAdmin,
		Status:    newUser.Status,
	}
	year, month, day := newUser.CreatedAt.Date()
	registerRsp.CreatedAt = fmt.Sprintf("%d.%d.%d", year, month, day)

	return registerRsp, nil
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
	if err := u.uow.Transaction(func(tx repository.UnitOfWork) error {
		// 1. 在事务内更新用户信息
		if err := tx.UserRepo().UpdateUserInfo(ctx, user); err != nil {
			return err
		}

		// 2. 在事务内同步更新 Session 表冗余字段（昵称/头像变更时保持一致性）
		sessionUpdates := make(map[string]interface{})
		if updateReq.Nickname != nil {
			sessionUpdates["receive_name"] = *updateReq.Nickname
		}
		if updateReq.Avatar != nil {
			sessionUpdates["avatar"] = *updateReq.Avatar
		}
		if len(sessionUpdates) > 0 {
			if err := tx.SessionRepo().UpdateByReceiveId(ctx, userId, sessionUpdates); err != nil {
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
		func() (interface{}, error) {
			user, err := u.uow.UserRepo().FindByUuid(ctx, targetId)
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
		func() (interface{}, error) {
			user, err := u.uow.UserRepo().FindByUuid(ctx, targetId)
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
