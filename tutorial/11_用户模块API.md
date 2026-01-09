# 11. 用户模块 API

> 本教程按 API 接口组织，每个接口展示完整的 Handler → Service → DAO 调用链。

---

## 1. 接口列表

| 接口 | 方法 | 路径 | 说明 |
|-----|------|------|------|
| 发送验证码 | POST | `/user/sendSmsCode` | 发送短信验证码 |
| 注册 | POST | `/register` | 用户注册 |
| 密码登录 | POST | `/login` | 密码登录 |
| 验证码登录 | POST | `/user/smsLogin` | 短信验证码登录 |
| 获取信息 | GET | `/user/getUserInfo?uuid=xxx` | 获取用户信息 |
| 更新信息 | POST | `/user/updateUserInfo` | 更新用户信息 |
| 获取用户列表 | GET | `/admin/user/list?ownerId=xxx` | 获取用户列表（管理员） |
| 启用用户 | POST | `/admin/user/able` | 启用用户（管理员） |
| 禁用用户 | POST | `/admin/user/disable` | 禁用用户（管理员） |
| 删除用户 | POST | `/admin/user/delete` | 删除用户（管理员） |
| 设置管理员 | POST | `/admin/user/setAdmin` | 设置管理员（管理员） |

---

## 2. 发送短信验证码

### Handler

```go
// POST /user/sendSmsCode
func SendSmsCodeHandler(c *gin.Context) {
	var req request.SendSmsCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.SendSmsCode(req.Telephone); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

```go
func (u *userInfoService) SendSmsCode(telephone string) error {
	return sms.VerificationCode(telephone)
}
```

### DAO (SMS Infrastructure)

```go
// 位置: internal/infrastructure/sms/auth_code_service.go
func VerificationCode(telephone string) error {
	// 1. 频率限制检查：Redis key = auth_code_<telephone>（key 存在则拒绝）
	// 2. 生成 6 位数字验证码
	// 3. 预写入 Redis（默认 1 分钟有效期，用于限流与占位）
	// 4. 调用阿里云短信 API（若失败会回滚删除 key）
	// 5. 未配置真实 AK/SK 时会走 Mock 模式（仅写入 Redis 并打印验证码）
}
```

---

## 3. 用户注册

### Handler

```go
// POST /register
func RegisterHandler(c *gin.Context) {
	var req request.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.Register(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

```go
func (u *userInfoService) Register(registerReq request.RegisterRequest) (*respond.RegisterRespond, error) {
	// 1. 验证验证码
	key := "auth_code_" + registerReq.Telephone
	code, err := u.cache.Get(context.Background(), key)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}
	if code != registerReq.SmsCode {
		return nil, errorx.New(errorx.CodeInvalidParam, "验证码不正确，请重试")
	}
	if err := u.cache.Delete(context.Background(), key); err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 2. 检查用户是否存在
	if err := u.checkTelephoneExist(registerReq.Telephone); err != nil {
		return nil, err
	}

	// 3. 创建用户
	var newUser model.UserInfo
	newUser.Uuid = "U" + random.GetNowAndLenRandomString(11)
	newUser.Telephone = registerReq.Telephone
	newUser.RawPassword = registerReq.Password
	newUser.Nickname = registerReq.Nickname
	newUser.Avatar = "https://cube.elemecdn.com/0/88/03b0d39583f48206768a7534e55bcpng.png"
	newUser.CreatedAt = time.Now()
	newUser.Status = user_status_enum.NORMAL

	if err := u.repos.User.CreateUser(&newUser); err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 4. 构造响应
	registerRsp := &respond.RegisterRespond{
		Uuid:      newUser.Uuid,
		Telephone: newUser.Telephone,
		Nickname:  newUser.Nickname,
		// ...其他字段
	}
	return registerRsp, nil
}
```

### DAO

```go
// UserRepository.Create
func (r *userRepository) CreateUser(user *model.UserInfo) error {
	if err := r.db.Create(user).Error; err != nil {
		return wrapDBError(err, "创建用户")
	}
	return nil
}

// UserRepository.FindByTelephone
func (r *userRepository) FindByTelephone(telephone string) (*model.UserInfo, error) {
	var user model.UserInfo
	if err := r.db.First(&user, "telephone = ?", telephone).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询用户 telephone=%s", telephone)
	}
	return &user, nil
}
```

---

## 4. 密码登录

### Handler

```go
// POST /login
func LoginHandler(c *gin.Context) {
	var req request.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.Login(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

```go
func (u *userInfoService) Login(loginReq request.LoginRequest) (*respond.LoginRespond, error) {
	// 1. 查询用户
	user, err := u.repos.User.FindByTelephone(loginReq.Telephone)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在，请注册")
		}
		return nil, errorx.ErrServerBusy
	}

	// 2. 校验密码 (bcrypt)
	if !user.CheckPassword(loginReq.Password) {
		return nil, errorx.New(errorx.CodeInvalidPassword, "密码不正确，请重试")
	}

	// 3. 生成双 Token（Access + Refresh）
	accessToken, err := jwt.GenerateAccessToken(user.Uuid)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}
	refreshToken, tokenID, err := jwt.GenerateRefreshToken(user.Uuid)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 4. 将 Refresh Token 的 tokenID 存入缓存（用于单点互踢 / SSO）
	// key: user_token:<uuid>
	_ = u.cache.Set(context.Background(), "user_token:"+user.Uuid, tokenID, time.Hour*24*7)

	// 5. 构造响应
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
```

### DAO

```go
// UserRepository.FindByTelephone (同上)
```

---

## 5. 验证码登录

### Handler

```go
// POST /user/smsLogin
func SmsLoginHandler(c *gin.Context) {
	var req request.SmsLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.SmsLogin(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

```go
func (u *userInfoService) SmsLogin(req request.SmsLoginRequest) (*respond.LoginRespond, error) {
	// 1. 查找用户
	user, err := u.repos.User.FindByTelephone(req.Telephone)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在，请注册")
		}
		return nil, errorx.ErrServerBusy
	}

	// 2. 验证验证码
	key := "auth_code_" + req.Telephone
	code, err := myredis.GetKey(key)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}
	if code != req.SmsCode {
		return nil, errorx.New(errorx.CodeInvalidParam, "验证码不正确，请重试")
	}
	myredis.DelKeyIfExists(key)

	// 3. 生成双 Token，并写入缓存用于单点互踢
	accessToken, err := jwt.GenerateAccessToken(user.Uuid)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}
	refreshToken, tokenID, err := jwt.GenerateRefreshToken(user.Uuid)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}
	_ = u.cache.Set(context.Background(), "user_token:"+user.Uuid, tokenID, time.Hour*24*7)

	// 4. 构造响应
	loginRsp := &respond.LoginRespond{
		Uuid:      user.Uuid,
		Telephone: user.Telephone,
		Nickname:  user.Nickname,
		// ...其他字段
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
	return loginRsp, nil
}
```

### DAO

```go
// UserRepository.FindByTelephone (同上)
// Redis: myredis.GetKey, myredis.DelKeyIfExists
```

---

## 6. 获取用户信息

### Handler

```go
// GET /user/getUserInfo?uuid=xxx
func GetUserInfoHandler(c *gin.Context) {
	var req request.GetUserInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.GetUserInfo(req.Uuid)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

> **特点**: Cache-Aside 模式 + 异步回写

```go
func (u *userInfoService) GetUserInfo(uuid string) (*respond.GetUserInfoRespond, error) {
	key := "user_info_" + uuid

	// 1. 尝试从 Redis 缓存获取
	rspString, err := myredis.GetKey(key)
	if err == nil && rspString != "" {
		var rsp respond.GetUserInfoRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return &rsp, nil
		}
		zap.L().Error("Redis cache unmarshal failed", zap.Error(err))
	}

	// 2. 查询数据库
	user, err := u.repos.User.FindByUuid(uuid)
	if err != nil {
		if errorx.GetCode(err) == errorx.CodeNotFound {
			return nil, errorx.New(errorx.CodeUserNotExist, "用户不存在")
		}
		return nil, errorx.ErrServerBusy
	}

	// 3. 构造响应
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
			return
		}
		myredis.SetKeyEx(key, string(jsonData), time.Hour)
	}()

	return rsp, nil
}
```

### DAO

```go
// UserRepository.FindByUuid
func (r *userRepository) FindByUuid(uuid string) (*model.UserInfo, error) {
	var user model.UserInfo
	if err := r.db.First(&user, "uuid = ?", uuid).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询用户 uuid=%s", uuid)
	}
	return &user, nil
}
```

---

## 7. 更新用户信息

### Handler

```go
// POST /user/updateUserInfo
func UpdateUserInfoHandler(c *gin.Context) {
	var req request.UpdateUserInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.UpdateUserInfo(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 更新后异步清理缓存

```go
func (u *userInfoService) UpdateUserInfo(updateReq request.UpdateUserInfoRequest) error {
	user, err := u.repos.User.FindByUuid(updateReq.Uuid)
	if err != nil {
		return errorx.ErrServerBusy
	}

	// 更新字段
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
		return errorx.ErrServerBusy
	}

	// 异步清理缓存
	go func() {
		myredis.DelKeyIfExists("user_info_" + updateReq.Uuid)
	}()

	return nil
}
```

### DAO

```go
// UserRepository.UpdateUserInfo
func (r *userRepository) UpdateUserInfo(user *model.UserInfo) error {
	if err := r.db.Save(user).Error; err != nil {
		return wrapDBError(err, "更新用户信息")
	}
	return nil
}
```

---

## 8. 获取用户列表（管理员）

### Handler

```go
// GET /admin/user/list?ownerId=xxx
func GetUserInfoListHandler(c *gin.Context) {
	var req request.GetUserInfoListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.User.GetUserInfoList(req.OwnerId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

> **特点**: Slice 预分配 + 简化布尔赋值

```go
func (u *userInfoService) GetUserInfoList(ownerId string) ([]respond.GetUserListRespond, error) {
	users, err := u.repos.User.FindAllExcept(ownerId)
	if err != nil {
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
```

### DAO

```go
// UserRepository.FindAllExcept
func (r *userRepository) FindAllExcept(excludeUuid string) ([]model.UserInfo, error) {
	var users []model.UserInfo
	if err := r.db.Unscoped().Where("uuid != ?", excludeUuid).Find(&users).Error; err != nil {
		return nil, wrapDBError(err, "查询用户列表")
	}
	return users, nil
}
```

---

## 9. 启用用户（管理员）

### Handler

```go
// POST /admin/user/able
func AbleUsersHandler(c *gin.Context) {
	var req request.AbleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.AbleUsers(req.UuidList); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 批量操作

```go
func (u *userInfoService) AbleUsers(uuidList []string) error {
	if len(uuidList) == 0 {
		return nil
	}
	if err := u.repos.User.UpdateUserStatusByUuids(uuidList, user_status_enum.NORMAL); err != nil {
		return errorx.ErrServerBusy
	}
	return nil
}
```

### DAO

```go
// UserRepository.UpdateUserStatusByUuids
func (r *userRepository) UpdateUserStatusByUuids(uuids []string, status int8) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Model(&model.UserInfo{}).Where("uuid IN ?", uuids).Update("status", status).Error; err != nil {
		return wrapDBError(err, "批量更新用户状态")
	}
	return nil
}
```

---

## 10. 禁用用户（管理员）

### Handler

```go
// POST /admin/user/disable
func DisableUsersHandler(c *gin.Context) {
	var req request.AbleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.DisableUsers(req.UuidList); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 批量操作 + 异步缓存清理

```go
func (u *userInfoService) DisableUsers(uuidList []string) error {
	if len(uuidList) == 0 {
		return nil
	}

	// 1. 批量更新用户状态
	if err := u.repos.User.UpdateUserStatusByUuids(uuidList, user_status_enum.DISABLE); err != nil {
		return errorx.ErrServerBusy
	}

	// 2. 批量删除会话
	if err := u.repos.Session.SoftDeleteByUsers(uuidList); err != nil {
		return errorx.ErrServerBusy
	}

	// 3. 异步清除缓存（不阻塞主流程）
	u.cache.SubmitTask(func() {
		var patterns []string
		for _, uuid := range uuidList {
			patterns = append(patterns,
				"user_info_"+uuid,
				"direct_session_list_"+uuid+"*",
				"group_session_list_"+uuid+"*",
			)
		}
		_ = u.cache.DeleteByPatterns(context.Background(), patterns)
	})

	return nil
}
```

### DAO

```go
// UserRepository.UpdateUserStatusByUuids (同上)
// SessionRepository.SoftDeleteByUsers
```

---

## 11. 删除用户（管理员）

### Handler

```go
// POST /admin/user/delete
func DeleteUsersHandler(c *gin.Context) {
	var req request.AbleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.DeleteUsers(req.UuidList); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 事务 + 批量操作 + 异步缓存清理

```go
func (u *userInfoService) DeleteUsers(uuidList []string) error {
	if len(uuidList) == 0 {
		return nil
	}

	err := u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 1. 批量软删除用户
		if err := txRepos.User.SoftDeleteUserByUuids(uuidList); err != nil {
			return errorx.ErrServerBusy
		}

		// 2. 批量软删除会话
		if err := txRepos.Session.SoftDeleteByUsers(uuidList); err != nil {
			return errorx.ErrServerBusy
		}

		// 3. 批量软删除联系人关系
		if err := txRepos.Contact.SoftDeleteByUsers(uuidList); err != nil {
			return errorx.ErrServerBusy
		}

		// 4. 批量软删除联系人申请
		if err := txRepos.Apply.SoftDeleteByUsers(uuidList); err != nil {
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 5. 异步清除缓存（不阻塞主流程）
	u.cache.SubmitTask(func() {
		var patterns []string
		for _, uuid := range uuidList {
			patterns = append(patterns,
				"user_info_"+uuid,
				"direct_session_list_"+uuid+"*",
				"group_session_list_"+uuid+"*",
				"contact_relation:user:"+uuid+"*",
			)
		}
		_ = u.cache.DeleteByPatterns(context.Background(), patterns)
	})

	return nil
}
```

### DAO

```go
// UserRepository.SoftDeleteUserByUuids
func (r *userRepository) SoftDeleteUserByUuids(uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Where("uuid IN ?", uuids).Delete(&model.UserInfo{}).Error; err != nil {
		return wrapDBError(err, "批量删除用户")
	}
	return nil
}
```

---

## 12. 设置管理员（管理员）

### Handler

```go
// POST /admin/user/setAdmin
func SetAdminHandler(c *gin.Context) {
	var req request.AbleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.User.SetAdmin(req.UuidList, req.IsAdmin); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 批量操作 + 异步缓存清理

```go
func (u *userInfoService) SetAdmin(uuidList []string, isAdmin int8) error {
	if len(uuidList) == 0 {
		return nil
	}

	// 1. 批量更新管理员状态
	if err := u.repos.User.UpdateUserIsAdminByUuids(uuidList, isAdmin); err != nil {
		return errorx.ErrServerBusy
	}

	// 2. 异步批量清除用户信息缓存
	go func(uuids []string) {
		var patterns []string
		for _, uuid := range uuids {
			patterns = append(patterns, "user_info_"+uuid)
		}
		myredis.DelKeysWithPatterns(patterns)
	}(uuidList)

	return nil
}
```

### DAO

```go
// UserRepository.UpdateUserIsAdminByUuids
func (r *userRepository) UpdateUserIsAdminByUuids(uuids []string, isAdmin int8) error {
	if len(uuids) == 0 {
		return nil
	}
	if err := r.db.Model(&model.UserInfo{}).Where("uuid IN ?", uuids).Update("is_admin", isAdmin).Error; err != nil {
		return wrapDBError(err, "批量更新用户管理员状态")
	}
	return nil
}
```

---

## 优化特性总结

| 函数 | 事务 | 批量操作 | 缓存模式 | 异步清理 |
|------|------|----------|---------|---------|
| Register | - | - | - | - |
| Login | - | - | - | - |
| SmsLogin | - | - | - | - |
| GetUserInfo | - | - | ✅ Cache-Aside | ✅ |
| UpdateUserInfo | - | - | - | ✅ |
| GetUserInfoList | - | - | - | - |
| AbleUsers | - | ✅ | - | - |
| DisableUsers | - | ✅ | - | ✅ |
| DeleteUsers | ✅ | ✅ | - | ✅ |
| SetAdmin | - | ✅ | - | ✅ |
