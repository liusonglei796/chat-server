package user

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql/repository"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/internal/infrastructure/sms"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/user_info/user_status_enum"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/jwt"
	"kama_chat_server/pkg/util/random"
)

// userInfoService 用户业务逻辑实现
// 通过构造函数注入 Repository 依赖，不再使用全局 dao.Repos
type userInfoService struct {
	repos *repository.Repositories
}

// NewUserService 构造函数，注入所有依赖的 Repository
func NewUserService(repos *repository.Repositories) *userInfoService {
	return &userInfoService{repos: repos}
}

// checkTelephoneValid 检验电话是否有效
func (u *userInfoService) checkTelephoneValid(telephone string) bool {
	pattern := `^1([38][0-9]|14[579]|5[^4]|16[6]|7[1-35-8]|9[189])\d{8}$`
	match, err := regexp.MatchString(pattern, telephone)
	if err != nil {
		zap.L().Error(err.Error())
	}
	return match
}

// checkEmailValid 校验邮箱是否有效
func (u *userInfoService) checkEmailValid(email string) bool {
	pattern := `^[^\s@]+@[^\s@]+\.[^\s@]+$`
	match, err := regexp.MatchString(pattern, email)
	if err != nil {
		zap.L().Error(err.Error())
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
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
	}
	if !user.CheckPassword(password) {
		return nil, errorx.New(errorx.CodeInvalidPassword, "密码不正确，请重试")
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

	// 将 Refresh Token ID 存入 Redis，实现单点互踢
	redisKey := "user_token:" + user.Uuid
	if err := myredis.SetKeyEx(redisKey, tokenID, time.Duration(constants.REFRESH_TOKEN_EXPIRY_HOURS)*time.Hour); err != nil {
		zap.L().Error("存储 Token ID 到 Redis 失败", zap.Error(err))
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
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
	}

	key := "auth_code_" + req.Telephone
	code, err := myredis.GetKey(key)
	if err != nil {
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
	}
	if code != req.SmsCode {
		return nil, errorx.New(errorx.CodeInvalidParam, "验证码不正确，请重试")
	}
	if err := myredis.DelKeyIfExists(key); err != nil {
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
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

	// 将 Refresh Token ID 存入 Redis，实现单点互踢
	redisKey := "user_token:" + user.Uuid
	if err := myredis.SetKeyEx(redisKey, tokenID, time.Duration(constants.REFRESH_TOKEN_EXPIRY_HOURS)*time.Hour); err != nil {
		zap.L().Error("存储 Token ID 到 Redis 失败", zap.Error(err))
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
	return sms.VerificationCode(telephone)
}

// checkTelephoneExist 检查手机号是否存在
func (u *userInfoService) checkTelephoneExist(telephone string) error {
	_, err := u.repos.User.FindByTelephone(telephone)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			zap.L().Info("该电话不存在，可以注册")
			return nil
		}
		zap.L().Error(err.Error())
		return errorx.ErrServerBusy
	}
	zap.L().Info("该电话已经存在，注册失败")
	return errorx.New(errorx.CodeUserExist, "该电话已经存在，注册失败")
}

// Register 注册
func (u *userInfoService) Register(registerReq request.RegisterRequest) (*respond.RegisterRespond, error) {
	key := "auth_code_" + registerReq.Telephone
	code, err := myredis.GetKey(key)
	if err != nil {
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
	}
	if code != registerReq.SmsCode {
		return nil, errorx.New(errorx.CodeInvalidParam, "验证码不正确，请重试")
	}
	if err := myredis.DelKeyIfExists(key); err != nil {
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
	}

	// 判断电话是否已经被注册过了
	if err := u.checkTelephoneExist(registerReq.Telephone); err != nil {
		return nil, err
	}

	var newUser model.UserInfo
	newUser.Uuid = "U" + random.GetNowAndLenRandomString(11)
	newUser.Telephone = registerReq.Telephone
	newUser.RawPassword = registerReq.Password
	newUser.Nickname = registerReq.Nickname
	newUser.Avatar = "https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png"
	newUser.CreatedAt = time.Now()
	newUser.IsAdmin = u.checkUserIsAdminOrNot(newUser)
	newUser.Status = user_status_enum.NORMAL

	err = u.repos.User.Create(&newUser)
	if err != nil {
		zap.L().Error(err.Error())
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
func (u *userInfoService) UpdateUserInfo(updateReq request.UpdateUserInfoRequest) error {
	user, err := u.repos.User.FindByUuid(updateReq.Uuid)
	if err != nil {
		zap.L().Error(err.Error())
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
		zap.L().Error(err.Error())
		return errorx.ErrServerBusy
	}

	// 异步清理缓存
	go func() {
		if err := myredis.DelKeyIfExists("user_info_" + updateReq.Uuid); err != nil {
			zap.L().Error(err.Error())
		}
	}()

	return nil
}

// GetUserInfoList 获取用户列表除了ownerId之外 - 管理员
func (u *userInfoService) GetUserInfoList(ownerId string) ([]respond.GetUserListRespond, error) {
	users, err := u.repos.User.FindAllExcept(ownerId)
	if err != nil {
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
	}
	rsp := make([]respond.GetUserListRespond, 0, len(users))
	for _, user := range users {
		rp := respond.GetUserListRespond{
			Uuid:      user.Uuid,
			Telephone: user.Telephone,
			Nickname:  user.Nickname,
			Status:    user.Status,
			IsAdmin:   user.IsAdmin,
			IsDeleted: user.DeletedAt.Valid,
		}
		rsp = append(rsp, rp)
	}
	return rsp, nil
}

// AbleUsers 启用用户 (批量优化版本)
func (u *userInfoService) AbleUsers(uuidList []string) error {
	if len(uuidList) == 0 {
		return nil
	}
	if err := u.repos.User.UpdateUserStatusByUuids(uuidList, user_status_enum.NORMAL); err != nil {
		zap.L().Error(err.Error())
		return errorx.ErrServerBusy
	}
	return nil
}

// DisableUsers 禁用用户 (批量优化版本)
func (u *userInfoService) DisableUsers(uuidList []string) error {
	if len(uuidList) == 0 {
		return nil
	}

	// 1. 批量更新用户状态
	if err := u.repos.User.UpdateUserStatusByUuids(uuidList, user_status_enum.DISABLE); err != nil {
		zap.L().Error(err.Error())
		return errorx.ErrServerBusy
	}

	// 2. 批量删除会话
	if err := u.repos.Session.SoftDeleteByUsers(uuidList); err != nil {
		zap.L().Error(err.Error())
		return errorx.ErrServerBusy
	}

	// 3. 异步清除 Redis 缓存
	go func(uuids []string) {
		var patterns []string
		for _, uuid := range uuids {
			patterns = append(patterns,
				"user_info_"+uuid,
				"direct_session_list_"+uuid+"*",
				"group_session_list_"+uuid+"*",
			)
		}
		if err := myredis.DelKeysWithPatterns(patterns); err != nil {
			zap.L().Error("批量清除用户相关缓存失败", zap.Error(err))
		}
	}(uuidList)

	return nil
}

// DeleteUsers 删除用户 - 批量优化版本 (增加事务支持)
func (u *userInfoService) DeleteUsers(uuidList []string) error {
	if len(uuidList) == 0 {
		return nil
	}

	err := u.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 1. 批量软删除用户
		if err := txRepos.User.SoftDeleteUserByUuids(uuidList); err != nil {
			zap.L().Error("Batch delete users error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 2. 批量软删除会话
		if err := txRepos.Session.SoftDeleteByUsers(uuidList); err != nil {
			zap.L().Error("Batch delete sessions error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 3. 批量软删除联系人关系
		if err := txRepos.Contact.SoftDeleteByUsers(uuidList); err != nil {
			zap.L().Error("Batch delete contacts error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 4. 批量软删除联系人申请
		if err := txRepos.ContactApply.SoftDeleteByUsers(uuidList); err != nil {
			zap.L().Error("Batch delete contact applies error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 5. 异步清除 Redis 缓存 (不阻塞主流程)
	go func(uuids []string) {
		// 收集所有需要删除的缓存模式
		var patterns []string
		for _, uuid := range uuids {
			patterns = append(patterns,
				"user_info_"+uuid,
				"direct_session_list_"+uuid+"*",
				"group_session_list_"+uuid+"*",
				"contact_user_list_"+uuid+"*",
			)
		}
		if err := myredis.DelKeysWithPatterns(patterns); err != nil {
			zap.L().Error("批量清除用户相关缓存失败", zap.Error(err))
		}
	}(uuidList)

	return nil
}

// GetUserInfo 获取用户信息
func (u *userInfoService) GetUserInfo(uuid string) (*respond.GetUserInfoRespond, error) {
	key := "user_info_" + uuid

	// 1. 尝试从 Redis 缓存获取
	rspString, err := myredis.GetKey(key)
	if err == nil && rspString != "" {
		var rsp respond.GetUserInfoRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return &rsp, nil
		}
		// 如果反序列化失败，视同缓存失效，继续查库
		zap.L().Error("Redis cache unmarshal failed", zap.Error(err))
	}

	// 2. 缓存未命中或异常，查询数据库
	user, err := u.repos.User.FindByUuid(uuid)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		zap.L().Error(err.Error())
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
	go func() {
		jsonData, err := json.Marshal(rsp)
		if err != nil {
			zap.L().Error("JSON marshal failed", zap.Error(err))
			return
		}
		if err := myredis.SetKeyEx(key, string(jsonData), time.Hour); err != nil {
			zap.L().Error("Redis set key failed", zap.Error(err))
		}
	}()

	return rsp, nil
}

// SetAdmin 设置管理员 (批量优化)
func (u *userInfoService) SetAdmin(uuidList []string, isAdmin int8) error {
	if len(uuidList) == 0 {
		return nil
	}

	// 1. 批量更新管理员状态
	if err := u.repos.User.UpdateUserIsAdminByUuids(uuidList, isAdmin); err != nil {
		zap.L().Error(err.Error())
		return errorx.ErrServerBusy
	}

	// 2. 异步批量清除用户信息缓存
	go func(uuids []string) {
		var patterns []string
		for _, uuid := range uuids {
			patterns = append(patterns, "user_info_"+uuid)
		}
		if err := myredis.DelKeysWithPatterns(patterns); err != nil {
			zap.L().Error("批量清除用户缓存失败", zap.Error(err))
		}
	}(uuidList)

	return nil
}
