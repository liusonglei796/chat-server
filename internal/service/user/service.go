package user

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/internal/infrastructure/sms"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/user_info/user_status_enum"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/jwt"
)

// userInfoService 用户业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖
type userInfoService struct {
	repos      *mysql.Repositories
	cache      myredis.AsyncCacheService
	smsService sms.SmsService
	kickClient func(userId, reason string) // 踢人回调函数（解耦 chat 包）
}

// NewUserService 构造函数，注入所有依赖
// kickClient: 可选的踢人回调函数，用于登录时踢掉旧设备
func NewUserService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService, smsService sms.SmsService, kickClient func(userId, reason string)) *userInfoService {
	return &userInfoService{
		repos:      repos,
		cache:      cacheService,
		smsService: smsService,
		kickClient: kickClient,
	}
}

// checkTelephoneValid 检验电话是否有效
func (u *userInfoService) checkTelephoneValid(telephone string) bool {
	pattern := `^1([38][0-9]|14[579]|5[^4]|16[6]|7[1-35-8]|9[189])\d{8}$`
	match, err := regexp.MatchString(pattern, telephone)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
	return match
}

// checkEmailValid 校验邮箱是否有效
func (u *userInfoService) checkEmailValid(email string) bool {
	pattern := `^[^\s@]+@[^\s@]+\.[^\s@]+$`
	match, err := regexp.MatchString(pattern, email)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
	return match
}

// checkUserIsAdminOrNot 检验用户是否为管理员
func (u *userInfoService) checkUserIsAdminOrNot(user model.UserInfo) int8 {
	return user.IsAdmin
}

// Login 登录
func (u *userInfoService) Login(loginReq request.LoginRequest) (*respond.LoginRespond, error) {
	password := loginReq.Password
	var user *model.UserInfo
	user, err := u.repos.User.FindByTelephone(loginReq.Telephone)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在，请注册")
		}
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if !user.CheckPassword(password) {
		return nil, errorx.New(errorx.CodeInvalidPassword, "密码不正确，请重试")
	}

	// 踢掉旧设备（如果在线）
	if u.kickClient != nil {
		u.kickClient(user.Uuid, "您的账号已在其他设备登录")
	}

	// 生成双 Token
	accessToken, err := jwt.GenerateAccessToken(user.Uuid)
	if err != nil {
		zap.L().Error("生成 Access Token 失败", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	refreshToken, tokenID, err := jwt.GenerateRefreshToken(user.Uuid)
	if err != nil {
		zap.L().Error("生成 Refresh Token 失败", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 将 Refresh Token ID 存入缓存，实现单点互踢
	redisKey := "user_token:" + user.Uuid
	if err := u.cache.Set(context.Background(), redisKey, tokenID, time.Duration(constants.REFRESH_TOKEN_EXPIRY_HOURS)*time.Hour); err != nil {
		zap.L().Error("存储 Token ID 到缓存失败", zap.Error(err))
		// 不阻塞登录流程，仅记录日志
	}

	loginRsp := &respond.LoginRespond{
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

// SmsLogin 验证码登录
func (u *userInfoService) SmsLogin(req request.SmsLoginRequest) (*respond.LoginRespond, error) {
	user, err := u.repos.User.FindByTelephone(req.Telephone)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在，请注册")
		}
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	key := "auth_code_" + req.Telephone
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

	// 踢掉旧设备（如果在线）
	if u.kickClient != nil {
		u.kickClient(user.Uuid, "您的账号已在其他设备登录")
	}

	// 生成双 Token
	accessToken, err := jwt.GenerateAccessToken(user.Uuid)
	if err != nil {
		zap.L().Error("生成 Access Token 失败", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	refreshToken, tokenID, err := jwt.GenerateRefreshToken(user.Uuid)
	if err != nil {
		zap.L().Error("生成 Refresh Token 失败", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 将 Refresh Token ID 存入缓存，实现单点互踢
	redisKey := "user_token:" + user.Uuid
	if err := u.cache.Set(context.Background(), redisKey, tokenID, time.Duration(constants.REFRESH_TOKEN_EXPIRY_HOURS)*time.Hour); err != nil {
		zap.L().Error("存储 Token ID 到缓存失败", zap.Error(err))
	}

	loginRsp := &respond.LoginRespond{
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

// SendSmsCode 发送短信验证码 - 验证码登录
func (u *userInfoService) SendSmsCode(telephone string) error {
	return u.smsService.SendVerificationCode(telephone)
}

// checkTelephoneExist 检查手机号是否存在
func (u *userInfoService) checkTelephoneExist(telephone string) error {
	_, err := u.repos.User.FindByTelephone(telephone)
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
func (u *userInfoService) Register(registerReq request.RegisterRequest) (*respond.RegisterRespond, error) {
	key := "auth_code_" + registerReq.Telephone
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
	if err := u.checkTelephoneExist(registerReq.Telephone); err != nil {
		return nil, err
	}

	var newUser model.UserInfo
	newUser.Uuid = "U" + snowflake.GenerateIDString()
	newUser.Telephone = registerReq.Telephone
	newUser.RawPassword = registerReq.Password
	newUser.Nickname = registerReq.Nickname
	newUser.Avatar = "https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png"
	newUser.CreatedAt = time.Now()
	newUser.IsAdmin = u.checkUserIsAdminOrNot(newUser)
	newUser.Status = user_status_enum.NORMAL

	err = u.repos.User.CreateUser(&newUser)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	registerRsp := &respond.RegisterRespond{
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
func (u *userInfoService) UpdateUserInfo(userId string, updateReq request.UpdateUserInfoRequest) error {
	user, err := u.repos.User.FindByUuid(userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if updateReq.Email != "" {
		user.Email = updateReq.Email
	}
	if updateReq.Nickname != "" {
		user.Nickname = updateReq.Nickname
	}
	if updateReq.Birthday != "" {
		user.Birthday = updateReq.Birthday
	}
	if updateReq.Signature != "" {
		user.Signature = updateReq.Signature
	}
	if updateReq.Avatar != "" {
		user.Avatar = updateReq.Avatar
	}
	if err := u.repos.User.UpdateUserInfo(user); err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 异步清理缓存
	u.cache.SubmitTask(func() {
		if err := u.cache.Delete(context.Background(), "user_info_"+userId); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
	})

	return nil
}

// GetUserInfo 获取用户信息
// GetUserInfo 获取用户完整信息（仅限自己调用）
func (u *userInfoService) GetUserInfo(requesterId, targetId string) (*respond.GetUserInfoRespond, error) {
	// 权限校验: 只能查看自己的完整信息
	if requesterId != targetId {
		return nil, errorx.New(errorx.CodeForbidden, "无权查看他人详细信息")
	}

	key := "user_info_" + targetId

	// 1. 尝试从缓存获取
	rspString, err := u.cache.Get(context.Background(), key)
	if err == nil && rspString != "" {
		var rsp respond.GetUserInfoRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return &rsp, nil
		}
		// 如果反序列化失败，视同缓存失效，继续查库
		zap.L().Error("Cache unmarshal failed", zap.Error(err))
	}

	// 2. 缓存未命中或异常，查询数据库
	user, err := u.repos.User.FindByUuid(targetId)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 3. 构造响应对象
	rsp := &respond.GetUserInfoRespond{
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
	}

	// 4. 异步回写缓存
	u.cache.SubmitTask(func() {
		jsonData, err := json.Marshal(rsp)
		if err != nil {
			zap.L().Error("JSON marshal failed", zap.Error(err))
			return
		}
		if err := u.cache.Set(context.Background(), key, string(jsonData), time.Hour); err != nil {
			zap.L().Error("Cache set key failed", zap.Error(err))
		}
	})

	return rsp, nil
}

// GetPublicUserInfo 获取用户公开信息（查看他人）
func (u *userInfoService) GetPublicUserInfo(targetId string) (*respond.PublicUserInfoRespond, error) {
	user, err := u.repos.User.FindByUuid(targetId)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	return &respond.PublicUserInfoRespond{
		Uuid:      user.Uuid,
		Nickname:  user.Nickname,
		Avatar:    user.Avatar,
		Gender:    user.Gender,
		Birthday:  user.Birthday,
		Signature: user.Signature,
	}, nil
}
