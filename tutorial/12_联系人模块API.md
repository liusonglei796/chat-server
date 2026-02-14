# 12. 联系人模块 API

> 本教程按 API 接口组织，每个接口展示完整的 Handler → Service → DAO 调用链。

---

## 1. 接口列表

| 接口 | 方法 | 路径 | 说明 |
|-----|------|------|------|
| 获取好友列表 | GET | `/friend/list?userId=xxx` | 获取好友列表 |
| 获取好友信息 | GET | `/friend/info?friendId=xxx` | 获取好友详情 |
| 申请添加好友 | POST | `/friend/apply` | 发送好友申请 |
| 获取好友申请列表 | GET | `/friend/applyList?userId=xxx` | 待处理好友申请 |
| 通过好友申请 | POST | `/friend/passApply` | 同意好友申请 |
| 拒绝好友申请 | POST | `/friend/refuseApply` | 拒绝好友申请 |
| 删除好友 | POST | `/friend/delete` | 删除好友 |
| 拉黑好友 | POST | `/friend/black` | 拉黑好友 |
| 取消拉黑 | POST | `/friend/cancelBlack` | 解除拉黑 |
| 拉黑好友申请 | POST | `/friend/blackApply` | 拉黑好友申请 |
| 获取我加入的群 | GET | `/group/loadMyJoinedGroup?userId=xxx` | 获取已加入群列表（排除自己创建的） |
| 获取群聊详情 | GET | `/group/getGroupDetail?groupId=xxx` | 获取群聊详情（会话用） |
| 申请加入群 | POST | `/group/apply` | 发送入群申请 |
| 获取入群申请列表 | GET | `/group/applyList?groupId=xxx` | 待处理入群申请 |
| 通过入群申请 | POST | `/group/passApply` | 同意入群申请 |
| 拒绝入群申请 | POST | `/group/refuseApply` | 拒绝入群申请 |
| 拉黑入群申请 | POST | `/group/blackApply` | 拉黑入群申请 |

---

## 2. 获取好友列表

### Handler

```go
// GET /friend/list?userId=xxx
func GetUserListHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetUserList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

> **特点**: Redis Set 缓存好友 ID + 批量查询用户信息

```go
func (u *contactService) GetUserList(userId string) ([]respond.MyUserListRespond, error) {
	// 优化：使用 Redis Set 存储好友 ID（contact_relation:user:<uid>）
	cacheKey := "contact_relation:user:" + userId

	// 1. 尝试从缓存获取好友 ID 列表
	memberIds, err := u.cache.GetSetMembers(context.Background(), cacheKey)
	if err != nil || len(memberIds) == 0 {
		// 2. 缓存未命中：查库
		contactList, dbErr := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.USER)
		if dbErr != nil {
			return nil, errorx.ErrServerBusy
		}

		memberIds = make([]string, 0, len(contactList))
		for _, c := range contactList {
			memberIds = append(memberIds, c.ContactId)
		}

		// 3. 回写缓存（Set）
		if len(memberIds) > 0 {
			args := make([]interface{}, len(memberIds))
			for i, v := range memberIds {
				args[i] = v
			}
			_ = u.cache.AddToSet(context.Background(), cacheKey, args...)
		}
	}

	if len(memberIds) == 0 {
		return []respond.MyUserListRespond{}, nil
	}

	// 4. 批量查询用户信息
	users, err := u.repos.User.FindByUuids(memberIds)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 5. 组装结果
	rsp := make([]respond.MyUserListRespond, 0, len(users))
	for _, user := range users {
		rsp = append(rsp, respond.MyUserListRespond{
			UserId:   user.Uuid,
			UserName: user.Nickname,
			Avatar:   user.Avatar,
		})
	}
	return rsp, nil
}
```

### DAO

```go
// ContactRepository.FindByUserIdAndType
func (r *contactRepository) FindByUserIdAndType(userId string, contactType int8) ([]model.Contact, error) {
	var contacts []model.Contact
	if err := r.db.Where("user_id = ? AND contact_type = ?", userId, contactType).Find(&contacts).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询联系人列表 user_id=%s type=%d", userId, contactType)
	}
	return contacts, nil
}

// UserRepository.FindByUuids
func (r *userRepository) FindByUuids(uuids []string) ([]model.UserInfo, error) {
	var users []model.UserInfo
	if err := r.db.Where("uuid IN ?", uuids).Find(&users).Error; err != nil {
		return nil, wrapDBError(err, "批量查询用户")
	}
	return users, nil
}
```

---

## 3. 获取我加入的群组

### Handler

```go
// GET /group/loadMyJoinedGroup?userId=xxx
func LoadMyJoinedGroupHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetJoinedGroupsExcludedOwn(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

> **特点**: Redis Set 缓存群组 ID + 批量查询 + 过滤自己创建的群

```go
func (u *contactService) GetJoinedGroupsExcludedOwn(userId string) ([]respond.LoadMyJoinedGroupRespond, error) {
	cacheKey := "contact_relation:group:" + userId

	// 1. 从缓存取已加入群组 ID
	groupUuids, err := u.cache.GetSetMembers(context.Background(), cacheKey)
	if err != nil || len(groupUuids) == 0 {
		// 2. 缓存未命中：查库
		contactList, dbErr := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.GROUP)
		if dbErr != nil {
			return nil, errorx.ErrServerBusy
		}
		groupUuids = make([]string, 0, len(contactList))
		for _, c := range contactList {
			if len(c.ContactId) > 0 && c.ContactId[0] == 'G' {
				groupUuids = append(groupUuids, c.ContactId)
			}
		}
		// 3. 回写缓存（Set）
		if len(groupUuids) > 0 {
			args := make([]interface{}, len(groupUuids))
			for i, v := range groupUuids {
				args[i] = v
			}
			_ = u.cache.AddToSet(context.Background(), cacheKey, args...)
		}
	}

	if len(groupUuids) == 0 {
		return []respond.LoadMyJoinedGroupRespond{}, nil
	}

	// 4. 批量查群信息
	groups, err := u.repos.Group.FindByUuids(groupUuids)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 5. 过滤自己创建的群
	rsp := make([]respond.LoadMyJoinedGroupRespond, 0, len(groups))
	for _, g := range groups {
		if g.OwnerId != userId {
			rsp = append(rsp, respond.LoadMyJoinedGroupRespond{
				GroupId:   g.Uuid,
				GroupName: g.Name,
				Avatar:    g.Avatar,
			})
		}
	}
	return rsp, nil
}
```

### DAO

```go
// GroupRepository.FindByUuids
func (r *groupRepository) FindByUuids(uuids []string) ([]model.GroupInfo, error) {
	if len(uuids) == 0 {
		return []model.GroupInfo{}, nil
	}
	var groups []model.GroupInfo
	if err := r.db.Where("uuid IN ?", uuids).Find(&groups).Error; err != nil {
		return nil, wrapDBError(err, "批量查询群组")
	}
	return groups, nil
}
```

---

## 4. 获取联系人信息（好友 / 群聊）

### 4.1 获取好友信息（Friend）

#### Handler

```go
// GET /friend/info?friendId=xxx
func GetFriendInfoHandler(c *gin.Context) {
	var req request.GetFriendInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetFriendInfo(req.FriendId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

#### Service

> **特点**: Cache-Aside（复用 `user_info_<uuid>` 缓存）

```go
func (u *contactService) GetFriendInfo(friendId string) (respond.GetFriendInfoRespond, error) {
	if len(friendId) == 0 {
		return respond.GetFriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
	}

	cacheKey := "user_info_" + friendId
	if cachedStr, err := u.cache.Get(context.Background(), cacheKey); err == nil && cachedStr != "" {
		var userRsp respond.GetUserInfoRespond
		if json.Unmarshal([]byte(cachedStr), &userRsp) == nil {
			return respond.GetFriendInfoRespond{
				FriendId:        userRsp.Uuid,
				FriendName:      userRsp.Nickname,
				FriendAvatar:    userRsp.Avatar,
				FriendBirthday:  userRsp.Birthday,
				FriendEmail:     userRsp.Email,
				FriendPhone:     userRsp.Telephone,
				FriendGender:    userRsp.Gender,
				FriendSignature: userRsp.Signature,
			}, nil
		}
	}

	user, err := u.repos.User.FindByUuid(friendId)
	if err != nil {
		return respond.GetFriendInfoRespond{}, errorx.ErrServerBusy
	}
	if user.Status == user_status_enum.DISABLE {
		return respond.GetFriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "该用户处于禁用状态")
	}

	rsp := respond.GetFriendInfoRespond{
		FriendId:        user.Uuid,
		FriendName:      user.Nickname,
		FriendAvatar:    user.Avatar,
		FriendBirthday:  user.Birthday,
		FriendEmail:     user.Email,
		FriendPhone:     user.Telephone,
		FriendGender:    user.Gender,
		FriendSignature: user.Signature,
	}

	// 回写缓存（与用户模块共用结构）
	userCache := respond.GetUserInfoRespond{
		Uuid: user.Uuid, Telephone: user.Telephone, Nickname: user.Nickname,
		Avatar: user.Avatar, Birthday: user.Birthday, Email: user.Email,
		Gender: user.Gender, Signature: user.Signature,
		CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
		IsAdmin: user.IsAdmin, Status: user.Status,
	}
	if data, err := json.Marshal(userCache); err == nil {
		_ = u.cache.Set(context.Background(), cacheKey, string(data), time.Hour)
	}

	return rsp, nil
}
```

### 4.2 获取群聊详情（Group）

#### Handler

```go
// GET /group/getGroupDetail?groupId=xxx
func GetGroupDetailHandler(c *gin.Context) {
	var req request.GetGroupInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetGroupDetail(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

#### Service

> **特点**: Cache-Aside（复用 `group_info_<uuid>` 缓存）

```go
func (u *contactService) GetGroupDetail(groupId string) (respond.GetGroupDetailRespond, error) {
	if len(groupId) == 0 {
		return respond.GetGroupDetailRespond{}, errorx.New(errorx.CodeInvalidParam, "群聊ID不能为空")
	}

	cacheKey := "group_info_" + groupId
	if cachedStr, err := u.cache.Get(context.Background(), cacheKey); err == nil && cachedStr != "" {
		var groupRsp respond.GetGroupInfoRespond
		if json.Unmarshal([]byte(cachedStr), &groupRsp) == nil {
			return respond.GetGroupDetailRespond{
				GroupId:     groupRsp.Uuid,
				GroupName:   groupRsp.Name,
				GroupAvatar: groupRsp.Avatar,
				GroupNotice: groupRsp.Notice,
				MemberCnt:   groupRsp.MemberCnt,
				OwnerId:     groupRsp.OwnerId,
				AddMode:     groupRsp.AddMode,
			}, nil
		}
	}

	group, err := u.repos.Group.FindByUuid(groupId)
	if err != nil {
		return respond.GetGroupDetailRespond{}, errorx.ErrServerBusy
	}
	if group.Status == group_status_enum.DISABLE {
		return respond.GetGroupDetailRespond{}, errorx.New(errorx.CodeInvalidParam, "该群聊处于禁用状态")
	}

	rsp := respond.GetGroupDetailRespond{
		GroupId:     group.Uuid,
		GroupName:   group.Name,
		GroupAvatar: group.Avatar,
		GroupNotice: group.Notice,
		MemberCnt:   group.MemberCnt,
		OwnerId:     group.OwnerId,
		AddMode:     group.AddMode,
	}

	groupCache := respond.GetGroupInfoRespond{
		Uuid: group.Uuid, Name: group.Name, Notice: group.Notice,
		Avatar: group.Avatar, MemberCnt: group.MemberCnt,
		OwnerId: group.OwnerId, AddMode: group.AddMode,
		Status: group.Status, IsDeleted: group.DeletedAt.Valid,
	}
	if data, err := json.Marshal(groupCache); err == nil {
		_ = u.cache.Set(context.Background(), cacheKey, string(data), time.Hour)
	}
	return rsp, nil
}
```

---

## 5. 删除好友

### Handler

```go
// POST /friend/delete
func DeleteContactHandler(c *gin.Context) {
	var req request.DeleteContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.DeleteContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 事务 + Set 缓存增量更新 + 异步清理会话缓存

```go
func (u *contactService) DeleteContact(userId, contactId string) error {
	err := u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 1. 仅从“我的”联系人列表中移除对方（单向）
		if err := txRepos.Contact.SoftDelete(userId, contactId); err != nil {
			return errorx.ErrServerBusy
		}

		// 2. 删除“我的视角”下的会话
		session, err := txRepos.Session.FindBySendIdAndReceiveId(userId, contactId)
		if err == nil && session != nil {
			_ = txRepos.Session.SoftDeleteByUuids([]string{session.Uuid})
		}

		// 3. 清理申请记录（可选）
		_ = txRepos.Apply.SoftDelete(userId, contactId)
		return nil
	})
	if err != nil {
		return err
	}

	// 4. 异步清理缓存
	u.cache.SubmitTask(func() {
		_ = u.cache.RemoveFromSet(context.Background(), "contact_relation:user:"+userId, contactId)
		_ = u.cache.DeleteByPattern(context.Background(), "direct_session_list_"+userId)
	})

	return nil
}
```

### DAO

```go
// ContactRepository.SoftDelete
func (r *contactRepository) SoftDelete(userId, contactId string) error {
	if err := r.db.Where("user_id = ? AND contact_id = ?", userId, contactId).
		Delete(&model.Contact{}).Error; err != nil {
		return wrapDBErrorf(err, "删除联系人关系 user_id=%s contact_id=%s", userId, contactId)
	}
	return nil
}
```

---

## 6. 申请添加好友

### Handler

```go
// POST /friend/apply
func ApplyFriendHandler(c *gin.Context) {
	var req request.ApplyFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.ApplyFriend(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

```go
func (u *contactService) ApplyFriend(req request.ApplyFriendRequest) error {
	// 1. 校验目标用户是否存在且有效
	user, err := u.repos.User.FindByUuid(req.FriendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		return errorx.ErrServerBusy
	}
	if user.Status == user_status_enum.DISABLE {
		return errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
	}

	// 2. 检查是否已是好友
	relation, err := u.repos.Contact.FindByUserIdAndContactId(req.UserId, req.FriendId)
	if err == nil && relation != nil && relation.Status == contact_status_enum.NORMAL {
		return errorx.New(errorx.CodeInvalidParam, "你们已经是好友")
	}

	// 3. 获取或创建申请记录
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(req.UserId, req.FriendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			apply = &model.Apply{
				Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)),
				ApplicantId: req.UserId,
				TargetId:    req.FriendId,
				ContactType: contact_type_enum.USER,
				Status:      contact_apply_status_enum.PENDING,
				Message:     req.Message,
				LastApplyAt: time.Now(),
			}
			return u.repos.Apply.CreateApply(apply)
		}
		return errorx.ErrServerBusy
	}

	// 4. 黑名单校验 + 更新旧记录
	if apply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "对方已将你拉黑，无法发送申请")
	}
	apply.LastApplyAt = time.Now()
	apply.Status = contact_apply_status_enum.PENDING
	apply.Message = req.Message
	return u.repos.Apply.Update(apply)
}
```

### DAO

```go
// ApplyRepository.FindByApplicantIdAndTargetId
func (r *applyRepository) FindByApplicantIdAndTargetId(applicantId, targetId string) (*model.Apply, error) {
	var apply model.Apply
	if err := r.db.Where("applicant_id = ? AND target_id = ?", applicantId, targetId).
		First(&apply).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return &apply, nil
}

// ApplyRepository.CreateApply
func (r *applyRepository) CreateApply(apply *model.Apply) error {
	if err := r.db.Create(apply).Error; err != nil {
		return wrapDBError(err, "创建联系人申请")
	}
	return nil
}
```

---

## 7. 获取好友申请列表

### Handler

```go
// GET /friend/applyList?userId=xxx
func GetFriendApplyListHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetFriendApplyList(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

> **特点**: 批量查询 + Map 快速查找 + 预分配

```go
func (u *contactService) GetFriendApplyList(userId string) ([]respond.NewContactListRespond, error) {
	// 1. 查询待处理申请
	applyList, err := u.repos.Apply.FindByTargetIdPending(userId)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}
	if len(applyList) == 0 {
		return []respond.NewContactListRespond{}, nil
	}

	// 2. 批量查询申请人信息
	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}
	userList, err := u.repos.User.FindByUuids(userUuids)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 3. Map 快速查找
	userMap := make(map[string]model.UserInfo)
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

	// 4. 组装结果 (预分配)
	rsp := make([]respond.NewContactListRespond, 0, len(applyList))
	for _, apply := range applyList {
		user, ok := userMap[apply.ApplicantId]
		if !ok {
			continue
		}
		message := "申请理由：无"
		if apply.Message != "" {
			message = "申请理由：" + apply.Message
		}
		rsp = append(rsp, respond.NewContactListRespond{
			ApplicantId:   user.Uuid,
			ContactName:   user.Nickname,
			ContactAvatar: user.Avatar,
			Message:       message,
		})
	}
	return rsp, nil
}
```

### DAO

```go
// ApplyRepository.FindByTargetIdPending
func (r *applyRepository) FindByTargetIdPending(targetId string) ([]model.Apply, error) {
	var applies []model.Apply
	if err := r.db.Where("target_id = ? AND status = ?", targetId, contact_apply_status_enum.PENDING).
		Find(&applies).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询待处理申请 target_id=%s", targetId)
	}
	return applies, nil
}
```

---

## 8. 通过好友申请

### Handler

```go
// POST /friend/passApply
func PassFriendApplyHandler(c *gin.Context) {
	var req request.PassFriendApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.PassFriendApply(req.UserId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 事务 + 双向建立关系 + 异步缓存清理

```go
func (u *contactService) PassFriendApply(userId string, applicantId string) error {
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		return errorx.ErrServerBusy
	}

	err = u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 校验申请人状态
		user, err := txRepos.User.FindByUuid(applicantId)
		if err != nil {
			return errorx.ErrServerBusy
		}
		if user.Status == user_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
		}

		// 更新申请状态
		apply.Status = contact_apply_status_enum.AGREE
		if err := txRepos.Apply.Update(apply); err != nil {
			return err
		}

		// 双向建立联系人关系
		if err := txRepos.Contact.CreateContact(&model.Contact{
			UserId: userId, ContactId: applicantId,
			ContactType: contact_type_enum.USER, Status: contact_status_enum.NORMAL,
		}); err != nil {
			return err
		}
		if err := txRepos.Contact.CreateContact(&model.Contact{
			UserId: applicantId, ContactId: userId,
			ContactType: contact_type_enum.USER, Status: contact_status_enum.NORMAL,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 异步清理缓存
	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+applicantId)
	})
	return nil
}
```

---

## 9. 拉黑好友

### Handler

```go
// POST /friend/black
func BlackContactHandler(c *gin.Context) {
	var req request.BlackContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.BlackContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 事务 + 双向状态更新 + 异步缓存清理

```go
func (u *contactService) BlackContact(userId, contactId string) error {
	err := u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 1. 更新拉黑者状态为 BLACK
		if err := txRepos.Contact.UpdateStatus(userId, contactId, contact_status_enum.BLACK); err != nil {
			return errorx.ErrServerBusy
		}
		// 2. 更新被拉黑者状态为 BE_BLACK
		if err := txRepos.Contact.UpdateStatus(contactId, userId, contact_status_enum.BE_BLACK); err != nil {
			return errorx.ErrServerBusy
		}
		// 3. 双方会话软删除
		if err := txRepos.Session.SoftDeleteByUsers([]string{userId, contactId}); err != nil {
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "direct_session_list_"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "direct_session_list_"+contactId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+contactId)
	})

	return nil
}
```

### DAO

```go
// ContactRepository.UpdateStatus
func (r *contactRepository) UpdateStatus(userId, contactId string, status int8) error {
	if err := r.db.Model(&model.Contact{}).
		Where("user_id = ? AND contact_id = ?", userId, contactId).
		Update("status", status).Error; err != nil {
		return wrapDBErrorf(err, "更新联系人状态 user_id=%s contact_id=%s", userId, contactId)
	}
	return nil
}
```

---

## 10. 取消拉黑好友

### Handler

```go
// POST /friend/cancelBlack
func CancelBlackContactHandler(c *gin.Context) {
	var req request.BlackContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.CancelBlackContact(req.UserId, req.ContactId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 事务 + 双向状态恢复 + 异步缓存清理

```go
func (u *contactService) CancelBlackContact(userId, contactId string) error {
	// 1. 校验状态
	blackContact, err := u.repos.Contact.FindByUserIdAndContactId(userId, contactId)
	if err != nil {
		return errorx.ErrServerBusy
	}
	if blackContact.Status != contact_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "未拉黑该联系人，无需解除拉黑")
	}

	beBlackContact, err := u.repos.Contact.FindByUserIdAndContactId(contactId, userId)
	if err != nil {
		return errorx.ErrServerBusy
	}
	if beBlackContact.Status != contact_status_enum.BE_BLACK {
		return errorx.New(errorx.CodeInvalidParam, "该联系人未被拉黑，无需解除拉黑")
	}

	// 2. 事务恢复双方状态
	err := u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		txRepos.Contact.UpdateStatus(userId, contactId, contact_status_enum.NORMAL)
		txRepos.Contact.UpdateStatus(contactId, userId, contact_status_enum.NORMAL)
		return nil
	})

	if err != nil {
		return err
	}

	// 3. 异步清理缓存
	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+contactId)
	})

	return nil
}
```

---

## 优化特性总结

| 函数 | 事务 | 批量操作 | 缓存模式 | 异步清理 |
|------|------|----------|---------|---------|
| GetUserList | - | ✅ | ✅ Redis Set（好友ID） | ✅（回写/清理） |
| LoadMyJoinedGroup | - | ✅ | ✅ Redis Set（群ID） | ✅（回写/清理） |
| GetFriendInfo | - | - | ✅ Cache-Aside | ✅（回写） |
| GetGroupDetail | - | - | ✅ Cache-Aside | ✅（回写） |
| DeleteContact | ✅ | - | ✅ Set 增量更新 | ✅ |
| ApplyFriend | - | - | - | - |
| GetFriendApplyList | - | ✅ | - | - |
| PassFriendApply | ✅ | - | - | ✅ |
| BlackContact | ✅ | - | - | ✅ |
| CancelBlackContact | ✅ | - | - | ✅ |
