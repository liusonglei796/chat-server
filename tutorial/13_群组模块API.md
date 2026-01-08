# 13. 群组模块 API

> 本教程按 API 接口组织，每个接口展示完整的 Handler → Service → DAO 调用链。

---

## 1. 接口列表

| 接口 | 方法 | 路径 | 说明 |
|-----|------|------|------|
| 创建群组 | POST | `/group/createGroup` | 创建新群组 |
| 获取我的群组 | GET | `/group/loadMyGroup?user_id=xxx` | 获取我创建的群组 |
| 检查加群方式 | GET | `/group/checkGroupAddMode?group_id=xxx` | 查看群加群方式 |
| 直接进群 | POST | `/group/enterGroupDirectly` | 直接加入群组 |
| 退出群组 | POST | `/group/leaveGroup` | 退出群组 |
| 解散群聊 | POST | `/group/dismissGroup` | 解散群组（群主） |
| 获取群信息 | GET | `/group/getGroupInfo?group_id=xxx` | 获取群基本信息 |
| 获取群列表 | GET | `/group/getGroupInfoList?page=1` | 获取所有群组（管理员） |
| 更新群信息 | POST | `/group/updateGroupInfo` | 更新群资料 |
| 获取群成员 | GET | `/group/getGroupMemberList?group_id=xxx` | 获取群成员列表 |
| 移除群成员 | POST | `/group/removeGroupMembers` | 踢出群成员 |
| 删除群组 | POST | `/group/deleteGroups` | 删除其它群组（管理员） |
| 设置群状态 | POST | `/group/setGroupsStatus` | 启用/禁用群组（管理员） |

---

## 2. 创建群组

### Handler

```go
// POST /group/createGroup
func CreateGroupHandler(c *gin.Context) {
	var req request.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.CreateGroup(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 事务 + 多表插入 + 异步缓存清理

```go
func (g *groupInfoService) CreateGroup(groupReq request.CreateGroupRequest) error {
	group := model.GroupInfo{
		Uuid:      fmt.Sprintf("G%s", random.GetNowAndLenRandomString(11)),
		Name:      groupReq.Name,
		Notice:    groupReq.Notice,
		OwnerId:   groupReq.OwnerId,
		MemberCnt: 1,
		AddMode:   groupReq.AddMode,
		Avatar:    groupReq.Avatar,
		Status:    group_status_enum.NORMAL,
	}

	err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 1. 创建群组
		if err := txRepos.Group.Create(&group); err != nil {
			return errorx.ErrServerBusy
		}
		// 2. 创建群成员（群主）
		member := model.GroupMember{
			GroupUuid: group.Uuid, UserUuid: groupReq.OwnerId, Role: 3,
		}
		if err := txRepos.GroupMember.Create(&member); err != nil {
			return errorx.ErrServerBusy
		}
		// 3. 创建联系人关系
		contact := model.Contact{
			UserId: groupReq.OwnerId, ContactId: group.Uuid,
			ContactType: contact_type_enum.GROUP, Status: contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.Create(&contact); err != nil {
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 异步清理缓存
	go func() {
		myredis.DelKeysWithPattern("contact_mygroup_list_" + groupReq.OwnerId)
	}()

	return nil
}
```

### DAO

```go
// GroupRepository.Create
func (r *groupRepository) Create(group *model.GroupInfo) error {
	if err := r.db.Create(group).Error; err != nil {
		return wrapDBError(err, "创建群组")
	}
	return nil
}
```

---

## 3. 获取我创建的群聊

### Handler

```go
// GET /group/loadMyGroup?user_id=xxx
func LoadMyGroupHandler(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Group.LoadMyGroup(req.UserId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

> **特点**: Cache-Aside + 异步缓存回写 + Slice预分配

```go
func (g *groupInfoService) LoadMyGroup(userId string) ([]respond.LoadMyGroupRespond, error) {
	cacheKey := "contact_mygroup_list_" + userId

	// 1. 尝试从缓存获取
	rspString, err := myredis.GetKeyNilIsErr(cacheKey)
	if err == nil {
		var groupListRsp []respond.LoadMyGroupRespond
		if err := json.Unmarshal([]byte(rspString), &groupListRsp); err == nil {
			return groupListRsp, nil
		}
	}

	// 2. 查询数据库
	groupList, err := g.repos.Group.FindByOwnerId(userId)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 3. 构建返回结果
	groupListRsp := make([]respond.LoadMyGroupRespond, 0, len(groupList))
	for _, group := range groupList {
		groupListRsp = append(groupListRsp, respond.LoadMyGroupRespond{
			GroupId:   group.Uuid,
			GroupName: group.Name,
			Avatar:    group.Avatar,
		})
	}

	// 4. 回写缓存 (异步)
	go func() {
		rspBytes, _ := json.Marshal(groupListRsp)
		myredis.SetKeyEx(cacheKey, string(rspBytes), time.Minute*30)
	}()

	return groupListRsp, nil
}
```

---

## 4. 检查加群方式

### Handler

```go
// GET /group/checkGroupAddMode?group_id=xxx
func CheckGroupAddModeHandler(c *gin.Context) {
	var req request.CheckGroupAddModeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	addMode, err := service.Svc.Group.CheckGroupAddMode(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, addMode)
}
```

### Service

> **特点**: Cache-Aside + 结构体转换

```go
func (g *groupInfoService) CheckGroupAddMode(groupId string) (int8, error) {
	cacheKey := "group_info_" + groupId

	// 1. 尝试读取缓存
	rspString, err := myredis.GetKeyNilIsErr(cacheKey)
	if err == nil {
		var rsp respond.GetGroupInfoRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return rsp.AddMode, nil
		}
	}

	// 2. 查询数据库
	group, err := g.repos.Group.FindByUuid(groupId)
	if err != nil {
		return -1, errorx.ErrServerBusy
	}

	// 3. 构建缓存对象
	cacheRsp := respond.GetGroupInfoRespond{
		Uuid: group.Uuid, Name: group.Name, Notice: group.Notice,
		MemberCnt: group.MemberCnt, OwnerId: group.OwnerId,
		AddMode: group.AddMode, Status: group.Status, Avatar: group.Avatar,
		IsDeleted: false,
	}

	// 4. 异步回写缓存
	go func() {
		rspBytes, _ := json.Marshal(cacheRsp)
		myredis.SetKeyEx(cacheKey, string(rspBytes), time.Hour*24)
	}()

	return group.AddMode, nil
}
```

---

## 5. 直接进群

### Handler

```go
// POST /group/enterGroupDirectly
func EnterGroupDirectlyHandler(c *gin.Context) {
	var req request.EnterGroupDirectlyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.EnterGroupDirectly(req.GroupId, req.UserId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

```go
func (g *groupInfoService) EnterGroupDirectly(groupId, userId string) error {
	err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 1. 创建群成员
		member := model.GroupMember{
			GroupUuid: groupId, UserUuid: userId, Role: 1,
		}
		if err := txRepos.GroupMember.Create(&member); err != nil {
			return errorx.ErrServerBusy
		}
		// 2. 增加群人数
		if err := txRepos.Group.IncrementMemberCount(groupId); err != nil {
			return errorx.ErrServerBusy
		}
		// 3. 建立联系人关系
		newContact := model.Contact{
			UserId: userId, ContactId: groupId,
			ContactType: contact_type_enum.GROUP, Status: contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.Create(&newContact); err != nil {
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 异步清理缓存
	go func() {
		myredis.DelKeysWithPattern("group_session_list_" + groupId)
		myredis.DelKeysWithPattern("my_joined_group_list_" + userId)
		myredis.DelKeyIfExists("group_info_" + groupId)
	}()
	return nil
}
```

---

## 6. 退出群组

### Handler

```go
// POST /group/leaveGroup
func LeaveGroupHandler(c *gin.Context) {
	var req request.LeaveGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.LeaveGroup(req.UserId, req.GroupId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

```go
func (g *groupInfoService) LeaveGroup(userId string, groupId string) error {
	err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 1. 删除群成员
		txRepos.GroupMember.Delete(groupId, userId)
		// 2. 减少群人数
		txRepos.Group.DecrementMemberCount(groupId)
		// 3. 删除会话
		session, _ := txRepos.Session.FindBySendIdAndReceiveId(userId, groupId)
		if session != nil {
			txRepos.Session.SoftDeleteByUuids([]string{session.Uuid})
		}
		// 4. 删除联系人关系
		txRepos.Contact.SoftDelete(userId, groupId)
		txRepos.Apply.SoftDelete(userId, groupId)
		return nil
	})

	if err != nil {
		return err
	}

	// 异步清理缓存
	go func() {
		myredis.DelKeysWithPattern("group_session_list_" + userId)
		myredis.DelKeysWithPattern("my_joined_group_list_" + userId)
		myredis.DelKeyIfExists("group_info_" + groupId)
		myredis.DelKeyIfExists("group_memberlist_" + groupId)
	}()
	return nil
}
```

---

## 7. 解散群聊（群主）

### Handler

```go
// POST /group/dismissGroup
func DismissGroupHandler(c *gin.Context) {
	var req request.DismissGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.DismissGroup(req.OwnerId, req.GroupId); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 繁重的级联删除（成员、群组、会话、联系人、申请）+ 精确缓存清理

```go
func (g *groupInfoService) DismissGroup(ownerId, groupId string) error {
	var memberIds []string

	err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 收集成员ID (用于缓存清理)
		contacts, _ := txRepos.Contact.FindUsersByContactId(groupId)
		for _, c := range contacts {
			memberIds = append(memberIds, c.UserId)
		}

		// 事务内级联删除
		txRepos.GroupMember.DeleteByGroupUuid(groupId)
		txRepos.Group.SoftDeleteByUuids([]string{groupId})
		txRepos.Session.SoftDeleteByUsers([]string{groupId})
		txRepos.Contact.SoftDeleteByUsers([]string{groupId})
		txRepos.Apply.SoftDeleteByUsers([]string{groupId})
		return nil
	})

	if err != nil {
		return err
	}

	// 精确清理缓存
	go func(members []string) {
		myredis.DelKeysWithPattern("contact_mygroup_list_" + ownerId)
		myredis.DelKeysWithPattern("group_session_list_" + ownerId)

		for _, memberId := range members {
			myredis.DelKeysWithPattern("my_joined_group_list_" + memberId)
			myredis.DelKeysWithPattern("group_session_list_" + memberId)
		}

		myredis.DelKeyIfExists("group_info_" + groupId)
		myredis.DelKeyIfExists("group_memberlist_" + groupId)
	}(memberIds)

	return nil
}
```

---

## 8. 获取群聊详情

### Handler

```go
// GET /group/getGroupInfo
func GetGroupInfoHandler(c *gin.Context) {
	var req request.GetGroupInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Group.GetGroupInfo(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

```go
func (g *groupInfoService) GetGroupInfo(groupId string) (*respond.GetGroupInfoRespond, error) {
	cacheKey := "group_info_" + groupId

	rspString, err := myredis.GetKeyNilIsErr(cacheKey)
	if err == nil {
		var rsp respond.GetGroupInfoRespond
		if json.Unmarshal([]byte(rspString), &rsp) == nil {
			return &rsp, nil
		}
	}

	group, err := g.repos.Group.FindByUuid(groupId)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	rsp := &respond.GetGroupInfoRespond{
		Uuid: group.Uuid, Name: group.Name, Notice: group.Notice,
		Avatar: group.Avatar, MemberCnt: group.MemberCnt,
		OwnerId: group.OwnerId, AddMode: group.AddMode, Status: group.Status,
		IsDeleted: group.DeletedAt.Valid,
	}

	go func() {
		data, _ := json.Marshal(rsp)
		myredis.SetKeyEx(cacheKey, string(data), time.Hour*24)
	}()

	return rsp, nil
}
```

---

## 9. 获取群成员列表

### Handler

```go
// GET /group/getGroupMemberList
func GetGroupMemberListHandler(c *gin.Context) {
	var req request.GetGroupMemberListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := service.Svc.Group.GetGroupMemberList(req.GroupId)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

```go
func (g *groupInfoService) GetGroupMemberList(groupId string) ([]respond.GetGroupMemberListRespond, error) {
	cacheKey := "group_memberlist_" + groupId

	rspString, err := myredis.GetKeyNilIsErr(cacheKey)
	if err == nil {
		var rsp []respond.GetGroupMemberListRespond
		if json.Unmarshal([]byte(rspString), &rsp) == nil {
			return rsp, nil
		}
	}

	members, err := g.repos.GroupMember.FindMembersWithUserInfo(groupId)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	rspList := make([]respond.GetGroupMemberListRespond, 0, len(members))
	for _, m := range members {
		rspList = append(rspList, respond.GetGroupMemberListRespond{
			UserId: m.UserId, Nickname: m.Nickname, Avatar: m.Avatar,
		})
	}

	go func() {
		data, _ := json.Marshal(rspList)
		myredis.SetKeyEx(cacheKey, string(data), time.Hour*24)
	}()

	return rspList, nil
}
```

---

## 10. 删除群组（管理员）

### Handler

```go
// POST /group/deleteGroups
func DeleteGroupsHandler(c *gin.Context) {
	var req request.DeleteGroupsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := service.Svc.Group.DeleteGroups(req.UuidList); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

> **特点**: 批量操作 + 级联删除 + 批量异步缓存清理

```go
func (g *groupInfoService) DeleteGroups(uuidList []string) error {
	if len(uuidList) == 0 {
		return nil
	}

	// 1. 收集信息用于清理缓存
	groups, _ := g.repos.Group.FindByUuids(uuidList)
	ownerIds := make([]string, 0, len(groups))
	for _, grp := range groups {
		ownerIds = append(ownerIds, grp.OwnerId)
	}
	memberIds, _ := g.repos.GroupMember.GetMemberIdsByGroupUuids(uuidList)

	// 2. 事务批量删除
	g.repos.Transaction(func(txRepos *repository.Repositories) error {
		txRepos.GroupMember.DeleteByGroupUuids(uuidList)
		txRepos.Group.SoftDeleteByUuids(uuidList)
		txRepos.Session.SoftDeleteByUsers(uuidList)
		txRepos.Contact.SoftDeleteByUsers(uuidList)
		txRepos.Apply.SoftDeleteByUsers(uuidList)
		return nil
	})

	// 3. 异步清理缓存
	go func() {
		for _, ownerId := range ownerIds {
			myredis.DelKeysWithPattern("contact_mygroup_list_" + ownerId)
			myredis.DelKeysWithPattern("group_session_list_" + ownerId)
		}
		for _, memId := range memberIds {
			myredis.DelKeysWithPattern("my_joined_group_list_" + memId)
			myredis.DelKeysWithPattern("group_session_list_" + memId)
		}
		for _, grpId := range uuidList {
			myredis.DelKeyIfExists("group_info_" + grpId)
			myredis.DelKeyIfExists("group_memberlist_" + grpId)
		}
	}()

	return nil
}
```

---

## 优化特性总结

| 函数 | 事务 | 批量操作 | 缓存模式 | 异步清理/回写 |
|------|------|----------|---------|---------|
| CreateGroup | ✅ | - | - | ✅ |
| LoadMyGroup | - | - | ✅ Cache-Aside | ✅ |
| CheckGroupAddMode | - | - | ✅ Cache-Aside | ✅ |
| EnterGroupDirectly | ✅ | - | - | ✅ |
| LeaveGroup | ✅ | - | - | ✅ |
| DismissGroup | ✅ | ✅ | - | ✅ |
| GetGroupInfo | - | - | ✅ Cache-Aside | ✅ |
| GetGroupInfoList | - | ✅ | - | - |
| UpdateGroupInfo | - | - | - | ✅ |
| GetGroupMemberList | - | - | ✅ Cache-Aside | ✅ |
| RemoveGroupMembers | ✅ | ✅ | - | ✅ |
| DeleteGroups | ✅ | ✅ | - | ✅ |
| SetGroupsStatus | - | ✅ | - | ✅ |
