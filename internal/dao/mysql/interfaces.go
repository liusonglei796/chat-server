// Package mysql 定义数据访问层接口和聚合结构
// 采用 Repository 模式将数据访问逻辑与业务逻辑分离
// 所有 Repository 接口在此文件定义，具体实现在各自的模块中
package mysql

import (
	"kama_chat_server/internal/model"
	"time"
)

// ==================== Repository 接口定义 ====================

// UserRepository 用户数据访问接口
// 提供用户的增删改查操作
type UserRepository interface {
	// FindByUuid 根据 UUID 查找用户
	FindByUuid(uuid string) (*model.UserInfo, error)
	// FindByTelephone 根据手机号查找用户
	FindByTelephone(telephone string) (*model.UserInfo, error)
	// FindByUuids 批量根据 UUID 查找用户
	FindByUuids(uuids []string) ([]model.UserInfo, error)
	// CreateUser 创建新用户
	CreateUser(user *model.UserInfo) error
	// UpdateUserInfo 更新用户信息
	UpdateUserInfo(user *model.UserInfo) error
	// UpdateUserStatusByUuids [管理员] 批量更新用户状态（启用/禁用）
	UpdateUserStatusByUuids(uuids []string, status int8) error
	// UpdateUserIsAdminByUuids [管理员] 批量设置用户管理员权限
	UpdateUserIsAdminByUuids(uuids []string, isAdmin int8) error
	// SoftDeleteUserByUuids [管理员] 批量软删除用户
	SoftDeleteUserByUuids(uuids []string) error
	// FindAllPaged [管理员] 分页查询用户列表（支持关键词搜索和状态筛选）
	FindAllPaged(page, pageSize int, keyword string, status *int8) ([]model.UserInfo, int64, error)
}

// GroupRepository 群组数据访问接口
// 提供群组的增删改查操作
type GroupRepository interface {
	// FindByUuid 根据 UUID 查找群组
	FindByUuid(uuid string) (*model.GroupInfo, error)
	// FindByOwnerIdPaged 根据群主 ID 分页查找群组
	// ownerId: 群主用户UUID
	// page: 页码（从1开始）
	// pageSize: 每页数量
	// 返回: 群组列表、总数、错误
	FindByOwnerIdPaged(ownerId string, page, pageSize int) ([]model.GroupInfo, int64, error)
	// FindByUuids 批量根据 UUID 查找群组
	FindByUuids(uuids []string) ([]model.GroupInfo, error)
	// GetGroupList [管理员] 分页获取群组列表
	GetGroupList(page, pageSize int) ([]model.GroupInfo, int64, error)
	// CreateGroup 创建新群组
	CreateGroup(group *model.GroupInfo) error
	// Update 更新群组信息
	Update(group *model.GroupInfo) error

	// UpdateStatusByUuids [管理员] 批量更新群组状态
	UpdateStatusByUuids(uuids []string, status int8) error
	// IncrementMemberCount 增加群成员数量（+1）
	IncrementMemberCount(uuid string) error
	// DecrementMemberCountBy 减少群成员数量（指定数量）
	DecrementMemberCountBy(uuid string, count int) error
	// SoftDeleteByUuids [管理员] 批量软删除群组
	SoftDeleteByUuids(uuids []string) error
}

// FriendshipRepository 好友关系数据访问接口
// 管理用户之间的好友关系
type FriendshipRepository interface {
	// FindByUserIdAndFriendId 根据用户ID和好友ID查找好友关系
	FindByUserIdAndFriendId(userId, friendId string) (*model.Friendship, error)
	// FindFriendsByUserId 分页查询用户的好友列表
	FindFriendsByUserId(userId string, page, pageSize int) ([]model.Friendship, int64, error)
	// CreateFriendship 创建好友关系
	CreateFriendship(fs *model.Friendship) error
	// IsFriend 判断两个用户是否互为好友
	IsFriend(userId1, userId2 string) (bool, error)
	// UpdateStatus 更新好友关系状态（正常/拉黑等）
	UpdateStatus(userId, friendId string, status int8) error
	// UpdateRemark 更新好友备注
	UpdateRemark(userId, friendId, remark string) error
	// SoftDelete 软删除好友关系（双向）
	SoftDelete(userId, friendId string) error
	// SoftDeleteByUsers [管理员] 批量软删除指定用户的所有好友关系
	SoftDeleteByUsers(userUuids []string) error
}

// SessionRepository 会话数据访问接口
// 管理聊天会话（用户之间或用户与群组之间）
type SessionRepository interface {
	// FindByUuid 根据会话UUID查找会话
	FindByUuid(uuid string) (*model.Session, error)
	// FindBySendIdAndReceiveId 根据发送者和接收者查找会话
	FindBySendIdAndReceiveId(sendId, receiveId string) (*model.Session, error)
	// FindBySendIdAndTypePaged 根据发送者ID和接收者类型前缀分页查找会话（传统分页）
	// receiveIdPrefix: "U" 表示私聊会话，"G" 表示群聊会话
	FindBySendIdAndTypePaged(sendId string, receiveIdPrefix string, page, pageSize int) ([]model.Session, int64, error)
	// FindBySendIdAndTypeCursor 根据发送者ID和接收者类型前缀游标分页查找会话（推荐）
	// receiveIdPrefix: "U" 表示私聊会话，"G" 表示群聊会话
	// cursor: 游标时间戳（上一页最后一条会话的 last_message_at Unix 时间戳）
	FindBySendIdAndTypeCursor(sendId string, receiveIdPrefix, cursor string, pageSize int) (*model.CursorPageSessionResult, error)
	// CreateSession 创建新会话
	CreateSession(session *model.Session) error
	// SoftDeleteByUuids 批量软删除会话
	SoftDeleteByUuids(uuids []string) error
	// SoftDeleteByUsers [管理员] 软删除指定用户的所有会话（用于注销账号）
	SoftDeleteByUsers(userUuids []string) error
	// UpdatePinStatus 更新会话置顶状态
	UpdatePinStatus(uuid string, isPinned bool) error
	// UpdateByReceiveId 根据接收者ID更新会话字段
	UpdateByReceiveId(receiveId string, updates map[string]interface{}) error
	// UpdateLastMessage 更新会话的最后一条消息信息
	UpdateLastMessage(sendId, receiveId, content string, msgType int8, msgTime time.Time) error
}

// MessageRepository 消息数据访问接口
// 管理聊天消息的存取
type MessageRepository interface {
	// FindByUserIdsPaged 根据两个用户ID查找私聊消息（分页）
	FindByUserIdsPaged(userOneId, userTwoId string, page, pageSize int) ([]model.Message, int64, error)
	// FindByUserIdsCursor 根据两个用户ID查找私聊消息（游标分页）
	// cursor: 游标时间戳（上一页最后一条消息的 created_at Unix 时间戳）
	// 返回: 游标分页结果（包含消息列表、下页游标、是否有更多）
	FindByUserIdsCursor(userOneId, userTwoId, cursor string, pageSize int) (*model.CursorPageMessageResult, error)
	// FindByGroupIdPaged 根据群组ID分页查找群聊消息
	FindByGroupIdPaged(groupId string, page, pageSize int) ([]model.Message, int64, error)
	// FindByGroupIdCursor 根据群组ID查找群聊消息（游标分页）
	// cursor: 游标时间戳（上一页最后一条消息的 created_at Unix 时间戳）
	// 返回: 游标分页结果（包含消息列表、下页游标、是否有更多）
	FindByGroupIdCursor(groupId, cursor string, pageSize int) (*model.CursorPageMessageResult, error)
	// FindByUuid 根据消息UUID查找消息
	FindByUuid(uuid string) (*model.Message, error)
	// UpdateStatus 更新消息状态
	UpdateStatus(uuid string, status int8) error
	// UpdateContent 更新消息内容和类型（用于撤回)
	UpdateContent(uuid, content string, msgType int8) error
	// Create 创建新消息
	Create(message *model.Message) error
}

// ApplyRepository 联系人申请数据访问接口
// 管理好友申请和入群申请
type ApplyRepository interface {
	// FindByApplicantIdAndTargetId 根据申请人和目标查找申请
	FindByApplicantIdAndTargetId(applicantId, targetId string) (*model.Apply, error)
	// FindByTargetIdPendingPaged 分页查找目标用户的待处理申请
	// targetId: 目标用户/群组 UUID
	// page: 页码（从1开始）
	// pageSize: 每页数量
	// 返回: 申请列表、总数、错误
	FindByTargetIdPendingPaged(targetId string, page, pageSize int) ([]model.Apply, int64, error)
	// CreateApply 创建新申请
	CreateApply(apply *model.Apply) error
	// Update 更新申请信息
	Update(apply *model.Apply) error
	// SoftDelete 软删除申请
	SoftDelete(applicantId, targetId string) error
	// SoftDeleteByUsers [管理员] 批量软删除指定用户的所有申请（用于注销账号）
	SoftDeleteByUsers(userUuids []string) error
}

// GroupMemberRepository 群成员数据访问接口
// 管理群组成员关系
// GroupMemberWithUserInfo 定义在 model 包中
type GroupMemberRepository interface {
	// FindByGroupUuid 根据群组UUID查找所有成员
	FindByGroupUuid(groupUuid string) ([]model.GroupMember, error)
	// FindMembersWithUserInfoPaged 分页查找群成员（含用户详细信息）
	FindMembersWithUserInfoPaged(groupUuid string, page, pageSize int) ([]model.GroupMemberWithUserInfo, int64, error)
	// FindByGroupAndUser 根据群组UUID和用户UUID查找成员
	FindByGroupAndUser(groupUuid, userUuid string) (*model.GroupMember, error)
	// CreateGroupMember 添加群成员
	CreateGroupMember(member *model.GroupMember) error

	// DeleteByGroupUuid 删除群组所有成员
	DeleteByGroupUuid(groupUuid string) error
	// DeleteByUserUuids 批量删除指定用户
	DeleteByUserUuids(groupUuid string, userUuids []string) error
	// DeleteByGroupUuids [管理员] 批量删除多个群组的所有成员
	DeleteByGroupUuids(groupUuids []string) error
	// GetMemberIdsByGroupUuids [管理员] 获取多个群组的所有成员ID
	GetMemberIdsByGroupUuids(groupUuids []string) ([]string, error)
	// FindGroupUuidsByUserPaged 根据用户UUID分页查找其加入的群组UUID
	FindGroupUuidsByUserPaged(userUuid string, page, pageSize int) ([]string, int64, error)
	// UpdateMuteUntil 更新群成员禁言截止时间
	UpdateMuteUntil(groupUuid, userUuid string, muteUntil *time.Time) error
}
