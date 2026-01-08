# 12. 联系人模块 API

> 本教程按 API 接口组织，每个接口展示完整的 Handler → Service → DAO 调用链。

---

## 1. 接口列表

| 接口 | 方法 | 路径 | 说明 |
|-----|------|------|------|
| 获取联系人列表 | GET | `/contact/getUserList?user_id=xxx` | 获取好友列表 |
| 获取联系人信息 | GET | `/contact/getContactInfo?contact_id=xxx` | 获取联系人详情 |
| 申请添加联系人 | POST | `/contact/applyContact` | 发送好友申请 |
| 获取申请列表 | GET | `/contact/getNewContactList?user_id=xxx` | 待处理申请列表 |
| 通过申请 | POST | `/contact/passApply` | 同意好友申请 |
| 拒绝申请 | POST | `/contact/refuseApply` | 拒绝好友申请 |
| 删除联系人 | POST | `/contact/deleteContact` | 删除好友 |
| 拉黑联系人 | POST | `/contact/blackContact` | 拉黑用户 |
| 取消拉黑 | POST | `/contact/cancelBlackContact` | 解除拉黑 |
| 拉黑申请 | POST | `/contact/blackApply` | 拉黑申请 |
| 获取我加入的群 | GET | `/contact/loadMyJoinedGroup?user_id=xxx` | 获取已加入群列表 |
| 获取群申请列表 | GET | `/contact/getAddGroupList?group_id=xxx` | 群加入申请列表 |

---

## 2. 获取联系人列表

### Handler

```go
// GET /contact/getUserList?user_id=xxx
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

> **特点**: Cache-Aside + 批量查询 + 防缓存穿透

```go
func (u *userContactService) GetUserList(userId string) ([]respond.MyUserListRespond, error) {
	cacheKey := "contact_user_list_" + userId

	// 1. 尝试从 Redis 获取
	rspString, err := myredis.GetKeyNilIsErr(cacheKey)
	if err == nil {
		var rsp []respond.MyUserListRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return rsp, nil
		}
	}

	// 2. 查询数据库
	contactList, err := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.USER)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 防缓存穿透
	if len(contactList) == 0 {
		u.setCache(cacheKey, []respond.MyUserListRespond{})
		return []respond.MyUserListRespond{}, nil
	}

	// 3. 批量查询用户信息
	uuids := make([]string, 0, len(contactList))
	for _, c := range contactList {
		uuids = append(uuids, c.ContactId)
	}
	users, err := u.repos.User.FindByUuids(uuids)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 4. 组装结果
	userListRsp := make([]respond.MyUserListRespond, 0, len(users))
	for _, user := range users {
		userListRsp = append(userListRsp, respond.MyUserListRespond{
			UserId:   user.Uuid,
			UserName: user.Nickname,
			Avatar:   user.Avatar,
		})
	}

	u.setCache(cacheKey, userListRsp)
	return userListRsp, nil
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
// GET /contact/loadMyJoinedGroup?user_id=xxx
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

> **特点**: Cache-Aside + 批量查询 + 过滤自己创建的群

```go
func (u *userContactService) GetJoinedGroupsExcludedOwn(userId string) ([]respond.LoadMyJoinedGroupRespond, error) {
	cacheKey := "my_joined_group_list_" + userId

	// 1. 尝试从缓存获取
	rspString, err := myredis.GetKeyNilIsErr(cacheKey)
	if err == nil {
		var rsp []respond.LoadMyJoinedGroupRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return rsp, nil
		}
	}

	// 2. 查询数据库
	contactList, err := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.GROUP)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	if len(contactList) == 0 {
		u.setCache(cacheKey, []respond.LoadMyJoinedGroupRespond{})
		return []respond.LoadMyJoinedGroupRespond{}, nil
	}

	// 3. 批量查询群组信息
	groupUuids := make([]string, 0, len(contactList))
	for _, contact := range contactList {
		if len(contact.ContactId) > 0 && contact.ContactId[0] == 'G' {
			groupUuids = append(groupUuids, contact.ContactId)
		}
	}
	groups, err := u.repos.Group.FindByUuids(groupUuids)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 4. 过滤自己创建的群
	groupListRsp := make([]respond.LoadMyJoinedGroupRespond, 0, len(groups))
	for _, group := range groups {
		if group.OwnerId != userId {
			groupListRsp = append(groupListRsp, respond.LoadMyJoinedGroupRespond{
				GroupId:   group.Uuid,
				GroupName: group.Name,
				Avatar:    group.Avatar,
			})
		}
	}

	u.setCache(cacheKey, groupListRsp)
	return groupListRsp, nil
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

## 4. 获取联系人信息

### Handler

```go
// GET /contact/getContactInfo?contact_id=xxx
func GetContactInfoHandler(c *gin.Context) {
	var req request.GetContactInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetContactInfo(req.ContactId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

> **特点**: Cache-Aside + 类型感知缓存（用户/群组）

```go
func (u *userContactService) GetContactInfo(contactId string) (respond.GetContactInfoRespond, error) {
	if len(contactId) == 0 {
		return respond.GetContactInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "ID不能为空")
	}

	// 1. 根据 ID 前缀区分类型
	var cacheKey string
	if contactId[0] == 'G' {
		cacheKey = "group_info_" + contactId
	} else {
		cacheKey = "user_info_" + contactId
	}

	// 2. 尝试从缓存获取
	cachedStr, err := myredis.GetKey(cacheKey)
	if err == nil && cachedStr != "" {
		// 根据类型反序列化
		if contactId[0] == 'G' {
			var groupRsp respond.GetGroupInfoRespond
			if json.Unmarshal([]byte(cachedStr), &groupRsp) == nil {
				return respond.GetContactInfoRespond{
					ContactId:        groupRsp.Uuid,
					ContactName:      groupRsp.Name,
					ContactAvatar:    groupRsp.Avatar,
					ContactNotice:    groupRsp.Notice,
					ContactAddMode:   groupRsp.AddMode,
					ContactMemberCnt: groupRsp.MemberCnt,
					ContactOwnerId:   groupRsp.OwnerId,
				}, nil
			}
		} else {
			var userRsp respond.GetUserInfoRespond
			if json.Unmarshal([]byte(cachedStr), &userRsp) == nil {
				return respond.GetContactInfoRespond{
					ContactId:        userRsp.Uuid,
					ContactName:      userRsp.Nickname,
					ContactAvatar:    userRsp.Avatar,
					ContactBirthday:  userRsp.Birthday,
					ContactEmail:     userRsp.Email,
					ContactPhone:     userRsp.Telephone,
					ContactGender:    userRsp.Gender,
					ContactSignature: userRsp.Signature,
				}, nil
			}
		}
	}

	// 3. 查询数据库并回写缓存
	if contactId[0] == 'G' {
		group, err := u.repos.Group.FindByUuid(contactId)
		if err != nil {
			return respond.GetContactInfoRespond{}, errorx.ErrServerBusy
		}
		// 回写缓存
		groupRsp := respond.GetGroupInfoRespond{
			Uuid: group.Uuid, Name: group.Name, Notice: group.Notice,
			Avatar: group.Avatar, MemberCnt: group.MemberCnt,
			OwnerId: group.OwnerId, AddMode: group.AddMode,
			Status: group.Status, IsDeleted: group.DeletedAt.Valid,
		}
		data, _ := json.Marshal(groupRsp)
		myredis.SetKeyEx(cacheKey, string(data), time.Hour)
		return respond.GetContactInfoRespond{
			ContactId: group.Uuid, ContactName: group.Name, ContactAvatar: group.Avatar,
			ContactNotice: group.Notice, ContactAddMode: group.AddMode,
			ContactMemberCnt: group.MemberCnt, ContactOwnerId: group.OwnerId,
		}, nil
	}

	user, err := u.repos.User.FindByUuid(contactId)
	if err != nil {
		return respond.GetContactInfoRespond{}, errorx.ErrServerBusy
	}
	// 回写缓存
	userRsp := respond.GetUserInfoRespond{
		Uuid: user.Uuid, Telephone: user.Telephone, Nickname: user.Nickname,
		Avatar: user.Avatar, Birthday: user.Birthday, Email: user.Email,
		Gender: user.Gender, Signature: user.Signature,
		CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
		IsAdmin: user.IsAdmin, Status: user.Status,
	}
	data, _ := json.Marshal(userRsp)
	myredis.SetKeyEx(cacheKey, string(data), time.Hour)
	return respond.GetContactInfoRespond{
		ContactId: user.Uuid, ContactName: user.Nickname, ContactAvatar: user.Avatar,
		ContactBirthday: user.Birthday, ContactEmail: user.Email,
		ContactPhone: user.Telephone, ContactGender: user.Gender,
		ContactSignature: user.Signature,
	}, nil
}
```

---

## 5. 删除联系人

### Handler

```go
// POST /contact/deleteContact
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

> **特点**: 事务 + 异步缓存清理

```go
func (u *userContactService) DeleteContact(userId, contactId string) error {
	err := u.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 1. 删除联系人关系
		if err := txRepos.Contact.SoftDelete(userId, contactId); err != nil {
			return errorx.ErrServerBusy
		}
		// 2. 删除会话
		session, err := txRepos.Session.FindBySendIdAndReceiveId(userId, contactId)
		if err == nil && session != nil {
			txRepos.Session.SoftDeleteByUuids([]string{session.Uuid})
		}
		// 3. 删除申请记录
		txRepos.Apply.SoftDelete(userId, contactId)
		return nil
	})

	if err != nil {
		return err
	}

	// 4. 异步清理缓存
	go func() {
		myredis.DelKeysWithPattern("contact_user_list_" + userId)
		myredis.DelKeysWithPattern("direct_session_list_" + userId)
	}()

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

## 6. 申请添加联系人

### Handler

```go
// POST /contact/applyContact
func ApplyContactHandler(c *gin.Context) {
	var req request.ApplyContactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.ApplyContact(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

```go
func (u *userContactService) ApplyContact(req request.ApplyContactRequest) error {
	// 1. 校验目标是否存在
	var contactType int8
	if req.ContactId[0] == 'U' {
		contactType = contact_type_enum.USER
		user, err := u.repos.User.FindByUuid(req.ContactId)
		if err != nil {
			return errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		if user.Status == user_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
		}
	} else if req.ContactId[0] == 'G' {
		contactType = contact_type_enum.GROUP
		group, err := u.repos.Group.FindByUuid(req.ContactId)
		if err != nil {
			return errorx.New(errorx.CodeNotFound, "该群聊不存在")
		}
		if group.Status == group_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该群聊已被禁用")
		}
	} else {
		return errorx.New(errorx.CodeInvalidParam, "非法ID格式")
	}

	// 2. 检查是否已是好友
	relation, _ := u.repos.Contact.FindByUserIdAndContactId(req.UserId, req.ContactId)
	if relation != nil && relation.Status == contact_status_enum.NORMAL {
		return errorx.New(errorx.CodeInvalidParam, "你们已经是好友/已在群中")
	}

	// 3. 获取或创建申请记录
	contactApply, err := u.repos.Apply.FindByApplicantIdAndTargetId(req.UserId, req.ContactId)
	if err != nil {
		if errorx.IsNotFound(err) {
			contactApply = &model.Apply{
				Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)),
				ApplicantId: req.UserId,
				TargetId:    req.ContactId,
				ContactType: contactType,
				Status:      contact_apply_status_enum.PENDING,
				Message:     req.Message,
				LastApplyAt: time.Now(),
			}
			return u.repos.Apply.Create(contactApply)
		}
		return errorx.ErrServerBusy
	}

	// 4. 更新已有申请
	if contactApply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "对方已将你拉黑")
	}
	contactApply.LastApplyAt = time.Now()
	contactApply.Status = contact_apply_status_enum.PENDING
	contactApply.Message = req.Message
	return u.repos.Apply.Update(contactApply)
}
```

### DAO

```go
// ApplyRepository.FindByApplicantIdAndTargetId
func (r *contactApplyRepository) FindByApplicantIdAndTargetId(applicantId, targetId string) (*model.Apply, error) {
	var apply model.Apply
	if err := r.db.Where("applicant_id = ? AND target_id = ?", applicantId, targetId).
		First(&apply).Error; err != nil {
		return nil, wrapDBErrorf(err, "查询申请 applicant_id=%s target_id=%s", applicantId, targetId)
	}
	return &apply, nil
}

// ApplyRepository.Create
func (r *contactApplyRepository) Create(apply *model.Apply) error {
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
// GET /contact/getNewContactList?user_id=xxx
func GetNewContactListHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Contact.GetNewContactList(req.UserId)
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
func (u *userContactService) GetNewContactList(userId string) ([]respond.NewContactListRespond, error) {
	// 1. 查询待处理申请
	contactApplyList, err := u.repos.Apply.FindByTargetIdPending(userId)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}
	if len(contactApplyList) == 0 {
		return []respond.NewContactListRespond{}, nil
	}

	// 2. 批量查询申请人信息
	userUuids := make([]string, 0, len(contactApplyList))
	for _, apply := range contactApplyList {
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
	rsp := make([]respond.NewContactListRespond, 0, len(contactApplyList))
	for _, apply := range contactApplyList {
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
func (r *contactApplyRepository) FindByTargetIdPending(targetId string) ([]model.Apply, error) {
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
// POST /contact/passApply
func PassApplyHandler(c *gin.Context) {
	var req request.PassApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Contact.PassApply(req.TargetId, req.ApplicantId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 事务 + 双向建立关系 + 异步缓存清理

```go
func (u *userContactService) PassApply(targetId, applicantId string) error {
	contactApply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, targetId)
	if err != nil {
		return errorx.ErrServerBusy
	}

	err = u.repos.Transaction(func(txRepos *repository.Repositories) error {
		if targetId[0] == 'U' {
			// 好友申请：双向建立关系
			contactApply.Status = contact_apply_status_enum.AGREE
			txRepos.Apply.Update(contactApply)

			txRepos.Contact.Create(&model.Contact{
				UserId: targetId, ContactId: applicantId,
				ContactType: contact_type_enum.USER, Status: contact_status_enum.NORMAL,
			})
			txRepos.Contact.Create(&model.Contact{
				UserId: applicantId, ContactId: targetId,
				ContactType: contact_type_enum.USER, Status: contact_status_enum.NORMAL,
			})
			return nil
		}

		// 入群申请
		contactApply.Status = contact_apply_status_enum.AGREE
		txRepos.Apply.Update(contactApply)
		txRepos.Contact.Create(&model.Contact{
			UserId: applicantId, ContactId: targetId,
			ContactType: contact_type_enum.GROUP, Status: contact_status_enum.NORMAL,
		})
		txRepos.GroupMember.Create(&model.GroupMember{
			GroupUuid: targetId, UserUuid: applicantId, Role: 1,
		})
		txRepos.Group.IncrementMemberCount(targetId)
		return nil
	})

	if err != nil {
		return err
	}

	// 异步清理缓存
	go func() {
		if targetId[0] == 'U' {
			myredis.DelKeysWithPattern("contact_user_list_" + targetId)
			myredis.DelKeysWithPattern("contact_user_list_" + applicantId)
		} else {
			myredis.DelKeysWithPattern("my_joined_group_list_" + applicantId)
			myredis.DelKeysWithPattern("group_info_" + targetId)
		}
	}()

	return nil
}
```

---

## 9. 拉黑联系人

### Handler

```go
// POST /contact/blackContact
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
func (u *userContactService) BlackContact(userId, contactId string) error {
	err := u.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 1. 更新拉黑者状态为 BLACK
		txRepos.Contact.UpdateStatus(userId, contactId, contact_status_enum.BLACK)
		// 2. 更新被拉黑者状态为 BE_BLACK
		txRepos.Contact.UpdateStatus(contactId, userId, contact_status_enum.BE_BLACK)
		// 3. 双方会话软删除
		txRepos.Session.SoftDeleteByUsers([]string{userId, contactId})
		return nil
	})

	if err != nil {
		return err
	}

	go func() {
		myredis.DelKeysWithPattern("direct_session_list_" + userId)
		myredis.DelKeysWithPattern("direct_session_list_" + contactId)
		myredis.DelKeysWithPattern("contact_user_list_" + userId)
		myredis.DelKeysWithPattern("contact_user_list_" + contactId)
	}()

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

## 10. 取消拉黑联系人

### Handler

```go
// POST /contact/cancelBlackContact
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
func (u *userContactService) CancelBlackContact(userId, contactId string) error {
	// 1. 校验状态
	blackContact, _ := u.repos.Contact.FindByUserIdAndContactId(userId, contactId)
	if blackContact == nil || blackContact.Status != contact_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "未拉黑该联系人")
	}

	// 2. 事务恢复双方状态
	err := u.repos.Transaction(func(txRepos *repository.Repositories) error {
		txRepos.Contact.UpdateStatus(userId, contactId, contact_status_enum.NORMAL)
		txRepos.Contact.UpdateStatus(contactId, userId, contact_status_enum.NORMAL)
		return nil
	})

	if err != nil {
		return err
	}

	// 3. 异步清理缓存
	go func() {
		myredis.DelKeysWithPattern("contact_user_list_" + userId)
		myredis.DelKeysWithPattern("contact_user_list_" + contactId)
	}()

	return nil
}
```

---

## 优化特性总结

| 函数 | 事务 | 批量操作 | 缓存模式 | 异步清理 |
|------|------|----------|---------|---------|
| GetUserList | - | ✅ | ✅ Cache-Aside | - |
| LoadMyJoinedGroup | - | ✅ | ✅ Cache-Aside | - |
| GetContactInfo | - | - | ✅ Cache-Aside | - |
| DeleteContact | ✅ | - | - | ✅ |
| ApplyContact | - | - | - | - |
| GetNewContactList | - | ✅ | - | - |
| GetAddGroupList | - | ✅ | - | - |
| PassApply | ✅ | - | - | ✅ |
| RefuseApply | - | - | - | - |
| BlackContact | ✅ | - | - | ✅ |
| CancelBlackContact | ✅ | - | - | ✅ |
