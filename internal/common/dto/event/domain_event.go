// Package event 定义跨服务领域事件 DTO 与类型常量
// 事件经 outbox → Kafka domain_events 在服务间传递
package event

const (
	EventGroupCreated   = "group_created"
	EventGroupJoined    = "group_joined"
	EventGroupDismissed = "group_dismissed"
	EventGroupApplyPassed = "group_apply_passed"
	EventFriendApplyPassed = "friend_apply_passed"
	EventFriendBlacked   = "friend_blacked"
	EventFriendDeleted   = "friend_deleted"
	EventGroupMemberRemoved = "group_member_removed"
	EventGroupMemberLeft    = "group_member_left"
)

// GroupCreatedEvent 建群事件（relation → message）
type GroupCreatedEvent struct {
	GroupId     string `json:"group_id"`
	OwnerId     string `json:"owner_id"`
	GroupName   string `json:"group_name"`
	GroupAvatar string `json:"group_avatar"`
}

// GroupJoinedEvent 入群事件（relation → message）
type GroupJoinedEvent struct {
	GroupId     string `json:"group_id"`
	UserId      string `json:"user_id"`
	GroupName   string `json:"group_name"`
	GroupAvatar string `json:"group_avatar"`
}

// GroupDismissedEvent 解散群事件（relation → message）
type GroupDismissedEvent struct {
	GroupId string `json:"group_id"`
}

// GroupApplyPassedEvent 同意加群事件（apply → group）
type GroupApplyPassedEvent struct {
	GroupId string `json:"group_id"`
	UserId  string `json:"user_id"`
}

// FriendApplyPassedEvent 同意加好友事件（apply → friendship）
type FriendApplyPassedEvent struct {
	UserId   string `json:"user_id"`
	FriendId string `json:"friend_id"`
}

// FriendBlackedEvent 拉黑好友事件（friendship → message，message 服务软删双方私聊会话）
type FriendBlackedEvent struct {
	UserId   string `json:"user_id"`
	FriendId string `json:"friend_id"`
}

// FriendDeletedEvent 删除好友事件（friendship → message，仅软删操作人单侧私聊会话）
type FriendDeletedEvent struct {
	UserId   string `json:"user_id"`   // 操作人（主动删除方）
	FriendId string `json:"friend_id"` // 被删除方
}

// GroupMemberRemovedEvent 移出群成员事件（group → message/gateway）
type GroupMemberRemovedEvent struct {
	GroupId     string   `json:"group_id"`
	OperatorId  string   `json:"operator_id"`
	MemberUuids []string `json:"member_uuids"`
}

// GroupMemberLeftEvent 主动退群事件（group → message/gateway）
type GroupMemberLeftEvent struct {
	GroupId string `json:"group_id"`
	UserId  string `json:"user_id"`
}
