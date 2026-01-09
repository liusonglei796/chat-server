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
| 获取群列表 | GET | `/admin/group/list?page=1&page_size=10` | 获取所有群组（管理员） |
| 更新群信息 | POST | `/group/updateGroupInfo` | 更新群资料 |
| 获取群成员 | GET | `/group/getGroupMemberList?group_id=xxx` | 获取群成员列表 |
| 移除群成员 | POST | `/group/removeGroupMembers` | 踢出群成员 |
| 删除群组 | POST | `/admin/group/delete` | 删除其它群组（管理员） |
| 设置群状态 | POST | `/admin/group/setStatus` | 启用/禁用群组（管理员） |

---

## 2. 创建群组

### Handler

```go
// POST /group/createGroup
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req request.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.CreateGroup(req); err != nil {
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
		if err := txRepos.Group.CreateGroup(&group); err != nil {
			return errorx.ErrServerBusy
		}
		// 2. 创建群成员（群主）
		member := model.GroupMember{
			GroupUuid: group.Uuid, UserUuid: groupReq.OwnerId, Role: 3,
		}
		if err := txRepos.GroupMember.CreateGroupMember(&member); err != nil {
			return errorx.ErrServerBusy
		}
		// 3. 创建联系人关系
		contact := model.Contact{
			UserId: groupReq.OwnerId, ContactId: group.Uuid,
			ContactType: contact_type_enum.GROUP, Status: contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&contact); err != nil {
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 异步清理缓存 (AsyncCacheService)
	g.cache.SubmitTask(func() {
		_ = g.cache.DeleteByPattern(context.Background(), "contact_mygroup_list_"+groupReq.OwnerId+"*")
	})

	return nil
}
```

### DAO

```go
// GroupRepository.CreateGroup
func (r *groupRepository) CreateGroup(group *model.GroupInfo) error {
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
func (h *GroupHandler) LoadMyGroup(c *gin.Context) {
	var req request.OwnlistRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.groupSvc.LoadMyGroup(req.UserId)
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
	rspString, err := g.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
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
	g.cache.SubmitTask(func() {
		rspBytes, err := json.Marshal(groupListRsp)
		if err != nil {
			return
		}
		_ = g.cache.Set(context.Background(), cacheKey, string(rspBytes), time.Minute*30)
	})

	return groupListRsp, nil
}
```

---

## 4. 检查加群方式

### Handler

```go
// GET /group/checkGroupAddMode?group_id=xxx
func (h *GroupHandler) CheckGroupAddMode(c *gin.Context) {
	var req request.CheckGroupAddModeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	addMode, err := h.groupSvc.CheckGroupAddMode(req.GroupId)
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
	rspString, err := g.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
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
	g.cache.SubmitTask(func() {
		rspBytes, err := json.Marshal(cacheRsp)
		if err != nil {
			return
		}
		_ = g.cache.Set(context.Background(), cacheKey, string(rspBytes), time.Hour*24)
	})

	return group.AddMode, nil
}
```

---

## 5. 直接进群

### Handler

```go
// POST /group/enterGroupDirectly
func (h *GroupHandler) EnterGroupDirectly(c *gin.Context) {
	var req request.EnterGroupDirectlyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.EnterGroupDirectly(req.GroupId, req.UserId); err != nil {
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
		if err := txRepos.GroupMember.CreateGroupMember(&member); err != nil {
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
		if err := txRepos.Contact.CreateContact(&newContact); err != nil {
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 异步清理缓存
	g.cache.SubmitTask(func() {
		_ = g.cache.DeleteByPattern(context.Background(), "group_session_list_"+groupId+"*")
		_ = g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+userId+"*")
		_ = g.cache.Delete(context.Background(), "group_info_"+groupId)
	})
	return nil
}
```

---

## 6. 退出群组

### Handler

```go
// POST /group/leaveGroup
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	var req request.LeaveGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.LeaveGroup(req.UserId, req.GroupId); err != nil {
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
		if err := txRepos.GroupMember.DeleteByUserUuids(groupId, []string{userId}); err != nil {
			return errorx.ErrServerBusy
		}
		// 2. 减少群人数
		if err := txRepos.Group.DecrementMemberCountBy(groupId, 1); err != nil {
			return errorx.ErrServerBusy
		}
		// 3. 删除会话（如存在）
		session, _ := txRepos.Session.FindBySendIdAndReceiveId(userId, groupId)
		if session != nil {
			_ = txRepos.Session.SoftDeleteByUuids([]string{session.Uuid})
		}
		// 4. 删除联系人关系
		if err := txRepos.Contact.SoftDelete(userId, groupId); err != nil {
			return errorx.ErrServerBusy
		}
		if err := txRepos.Apply.SoftDelete(userId, groupId); err != nil {
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 异步清理缓存
	g.cache.SubmitTask(func() {
		_ = g.cache.DeleteByPattern(context.Background(), "group_session_list_"+userId+"*")
		_ = g.cache.RemoveFromSet(context.Background(), "contact_relation:group:"+userId, groupId)
		_ = g.cache.Delete(context.Background(), "group_info_"+groupId)
		_ = g.cache.Delete(context.Background(), "group_memberlist_"+groupId)
	})
	return nil
}
```

---

## 7. 解散群聊（群主）

### Handler

```go
// POST /group/dismissGroup
func (h *GroupHandler) DismissGroup(c *gin.Context) {
	var req request.DismissGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.DismissGroup(req.OwnerId, req.GroupId); err != nil {
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
		contacts, err := txRepos.Contact.FindUsersByContactId(groupId)
		if err != nil {
			return errorx.ErrServerBusy
		}
		for _, c := range contacts {
			memberIds = append(memberIds, c.UserId)
		}

		// 事务内级联删除
		if err := txRepos.GroupMember.DeleteByGroupUuid(groupId); err != nil {
			return errorx.ErrServerBusy
		}
		if err := txRepos.Group.SoftDeleteByUuids([]string{groupId}); err != nil {
			return errorx.ErrServerBusy
		}
		if err := txRepos.Session.SoftDeleteByUsers([]string{groupId}); err != nil {
			return errorx.ErrServerBusy
		}
		if err := txRepos.Contact.SoftDeleteByUsers([]string{groupId}); err != nil {
			return errorx.ErrServerBusy
		}
		if err := txRepos.Apply.SoftDeleteByUsers([]string{groupId}); err != nil {
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 精确清理缓存
	g.cache.SubmitTask(func() {
		_ = g.cache.DeleteByPattern(context.Background(), "contact_mygroup_list_"+ownerId+"*")
		_ = g.cache.DeleteByPattern(context.Background(), "group_session_list_"+ownerId+"*")

		for _, memberId := range memberIds {
			_ = g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+memberId+"*")
			_ = g.cache.DeleteByPattern(context.Background(), "group_session_list_"+memberId+"*")
		}

		_ = g.cache.Delete(context.Background(), "group_info_"+groupId)
		_ = g.cache.Delete(context.Background(), "group_memberlist_"+groupId)
	})

	return nil
}
```


## 8. 获取群聊详情

### Handler

```go
// GET /group/getGroupInfo?group_id=xxx
func (h *GroupHandler) GetGroupInfo(c *gin.Context) {
	var req request.GetGroupInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.groupSvc.GetGroupInfo(req.GroupId)
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

	// 1. 尝试从缓存获取
	rspString, err := g.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
		var rsp respond.GetGroupInfoRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return &rsp, nil
		}
	}

	// 2. 查询数据库
	group, err := g.repos.Group.FindByUuid(groupId)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 3. 组装响应
	rsp := &respond.GetGroupInfoRespond{
		Uuid:      group.Uuid,
		Name:      group.Name,
		Notice:    group.Notice,
		Avatar:    group.Avatar,
		MemberCnt: group.MemberCnt,
		OwnerId:   group.OwnerId,
		AddMode:   group.AddMode,
		Status:    group.Status,
	}
	if group.DeletedAt.Valid {
		rsp.IsDeleted = true
	} else {
		rsp.IsDeleted = false
	}

	// 4. 回写缓存
	g.cache.SubmitTask(func() {
		data, err := json.Marshal(rsp)
		if err != nil {
			return
		}
		_ = g.cache.Set(context.Background(), cacheKey, string(data), time.Hour*24)
	})

	return rsp, nil
}
```

---

## 9. 获取群成员列表

### Handler

```go
// GET /group/getGroupMemberList?group_id=xxx
func (h *GroupHandler) GetGroupMemberList(c *gin.Context) {
	var req request.GetGroupMemberListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.groupSvc.GetGroupMemberList(req.GroupId)
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

	// 1. 尝试从缓存获取
	rspString, err := g.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
		var rsp []respond.GetGroupMemberListRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return rsp, nil
		}
	}

	// 2. 查库
	members, err := g.repos.GroupMember.FindMembersWithUserInfo(groupId)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	// 3. 组装响应
	rspList := make([]respond.GetGroupMemberListRespond, 0, len(members))
	for _, m := range members {
		rspList = append(rspList, respond.GetGroupMemberListRespond{
			UserId:   m.UserId,
			Nickname: m.Nickname,
			Avatar:   m.Avatar,
		})
	}

	// 4. 回写缓存
	g.cache.SubmitTask(func() {
		data, err := json.Marshal(rspList)
		if err != nil {
			return
		}
		_ = g.cache.Set(context.Background(), cacheKey, string(data), time.Hour*24)
	})

	return rspList, nil
}
```

---

## 10. 获取群列表（管理员）

### Handler

```go
// GET /admin/group/list?page=1&page_size=10
func (h *GroupHandler) GetGroupInfoList(c *gin.Context) {
	var req request.GetGroupListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	data, err := h.groupSvc.GetGroupInfoList(req)
	if err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, data)
}
```

### Service

```go
func (g *groupInfoService) GetGroupInfoList(req request.GetGroupListRequest) (*respond.GetGroupListWrapper, error) {
	groupList, total, err := g.repos.Group.GetGroupList(req.Page, req.PageSize)
	if err != nil {
		return nil, errorx.ErrServerBusy
	}

	rsp := make([]respond.GetGroupListRespond, 0, len(groupList))
	for _, group := range groupList {
		rsp = append(rsp, respond.GetGroupListRespond{
			Uuid:      group.Uuid,
			Name:      group.Name,
			OwnerId:   group.OwnerId,
			Status:    group.Status,
			IsDeleted: group.DeletedAt.Valid,
		})
	}

	return &respond.GetGroupListWrapper{List: rsp, Total: total}, nil
}
```

---

## 11. 删除群组（管理员）

### Handler

```go
// POST /admin/group/delete
func (h *GroupHandler) DeleteGroups(c *gin.Context) {
	var req request.DeleteGroupsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.DeleteGroups(req.UuidList); err != nil {
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

	groups, err := g.repos.Group.FindByUuids(uuidList)
	if err != nil {
		return errorx.ErrServerBusy
	}
	ownerIds := make([]string, 0, len(groups))
	for _, grp := range groups {
		ownerIds = append(ownerIds, grp.OwnerId)
	}

	memberIds, err := g.repos.GroupMember.GetMemberIdsByGroupUuids(uuidList)
	if err != nil {
		return errorx.ErrServerBusy
	}

	if err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
		if err := txRepos.GroupMember.DeleteByGroupUuids(uuidList); err != nil {
			return errorx.ErrServerBusy
		}
		if err := txRepos.Group.SoftDeleteByUuids(uuidList); err != nil {
			return errorx.ErrServerBusy
		}
		if err := txRepos.Session.SoftDeleteByUsers(uuidList); err != nil {
			return errorx.ErrServerBusy
		}
		if err := txRepos.Contact.SoftDeleteByUsers(uuidList); err != nil {
			return errorx.ErrServerBusy
		}
		if err := txRepos.Apply.SoftDeleteByUsers(uuidList); err != nil {
			return errorx.ErrServerBusy
		}
		return nil
	}); err != nil {
		return err
	}

	g.cache.SubmitTask(func() {
		for _, ownerId := range ownerIds {
			_ = g.cache.DeleteByPattern(context.Background(), "contact_mygroup_list_"+ownerId+"*")
			_ = g.cache.DeleteByPattern(context.Background(), "group_session_list_"+ownerId+"*")
		}
		for _, memId := range memberIds {
			_ = g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+memId+"*")
			_ = g.cache.DeleteByPattern(context.Background(), "group_session_list_"+memId+"*")
		}
		for _, grpId := range uuidList {
			_ = g.cache.Delete(context.Background(), "group_info_"+grpId)
			_ = g.cache.Delete(context.Background(), "group_memberlist_"+grpId)
		}
	})

	return nil
}
```

---

## 12. 设置群状态（管理员）

### Handler

```go
// POST /admin/group/setStatus
func (h *GroupHandler) SetGroupsStatus(c *gin.Context) {
	var req request.SetGroupsStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.SetGroupsStatus(req.UuidList, req.Status); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

```go
func (g *groupInfoService) SetGroupsStatus(uuidList []string, status int8) error {
	if len(uuidList) == 0 {
		return nil
	}

	if err := g.repos.Group.UpdateStatusByUuids(uuidList, status); err != nil {
		return errorx.ErrServerBusy
	}

	if status == group_status_enum.DISABLE {
		_ = g.repos.Session.SoftDeleteByUsers(uuidList)
	}

	g.cache.SubmitTask(func() {
		keys := make([]string, 0, len(uuidList))
		for _, uuid := range uuidList {
			keys = append(keys, "group_info_"+uuid)
		}
		_ = g.cache.DeleteByPatterns(context.Background(), keys)
	})

	return nil
}
```

---

## 13. 更新群信息

### Handler

```go
// POST /group/updateGroupInfo
func (h *GroupHandler) UpdateGroupInfo(c *gin.Context) {
	var req request.UpdateGroupInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.UpdateGroupInfo(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

```go
func (g *groupInfoService) UpdateGroupInfo(req request.UpdateGroupInfoRequest) error {
	group, err := g.repos.Group.FindByUuid(req.Uuid)
	if err != nil {
		return errorx.ErrServerBusy
	}

	if req.Name != "" {
		group.Name = req.Name
	}
	if req.AddMode != -1 {
		group.AddMode = req.AddMode
	}
	if req.Notice != "" {
		group.Notice = req.Notice
	}
	if req.Avatar != "" {
		group.Avatar = req.Avatar
	}

	if err := g.repos.Group.Update(group); err != nil {
		return errorx.ErrServerBusy
	}

	sessionUpdates := map[string]interface{}{
		"receive_name": group.Name,
		"avatar":       group.Avatar,
	}
	_ = g.repos.Session.UpdateByReceiveId(req.Uuid, sessionUpdates)

	g.cache.SubmitTask(func() {
		_ = g.cache.Delete(context.Background(), "group_info_"+req.Uuid)
	})

	return nil
}
```

---

## 14. 移除群成员

### Handler

```go
// POST /group/removeGroupMembers
func (h *GroupHandler) RemoveGroupMembers(c *gin.Context) {
	var req request.RemoveGroupMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleParamError(c, err)
		return
	}
	if err := h.groupSvc.RemoveGroupMembers(req); err != nil {
		HandleError(c, err)
		return
	}
	HandleSuccess(c, nil)
}
```

### Service

```go
func (g *groupInfoService) RemoveGroupMembers(req request.RemoveGroupMembersRequest) error {
	if len(req.UuidList) == 0 {
		return nil
	}

	for _, uuid := range req.UuidList {
		if req.OwnerId == uuid {
			return errorx.New(errorx.CodeInvalidParam, "不能移除群主")
		}
	}

	if err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
		if err := txRepos.GroupMember.DeleteByUserUuids(req.GroupId, req.UuidList); err != nil {
			return errorx.ErrServerBusy
		}
		if err := txRepos.Group.DecrementMemberCountBy(req.GroupId, len(req.UuidList)); err != nil {
			return errorx.ErrServerBusy
		}
		for _, uuid := range req.UuidList {
			_ = txRepos.Contact.SoftDelete(uuid, req.GroupId)
			_ = txRepos.Apply.SoftDelete(uuid, req.GroupId)
		}
		_ = txRepos.Session.SoftDeleteByUsers([]string{req.GroupId})
		return nil
	}); err != nil {
		return err
	}

	g.cache.SubmitTask(func() {
		for _, memId := range req.UuidList {
			_ = g.cache.DeleteByPattern(context.Background(), "group_session_list_"+memId+"*")
			_ = g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+memId+"*")
		}
		_ = g.cache.Delete(context.Background(), "group_info_"+req.GroupId)
		_ = g.cache.Delete(context.Background(), "group_memberlist_"+req.GroupId)
	})

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
| GetGroupMemberList | - | - | ✅ Cache-Aside | ✅ |
| GetGroupInfoList | - | ✅ | - | - |
| DeleteGroups | ✅ | ✅ | - | ✅ |
| SetGroupsStatus | - | ✅ | - | ✅ |
| UpdateGroupInfo | - | - | - | ✅ |
| RemoveGroupMembers | ✅ | ✅ | - | ✅ |
