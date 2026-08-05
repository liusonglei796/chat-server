// Package event 定义跨服务领域事件 DTO 与类型常量
// 事件经 outbox → Kafka domain_events 在服务间传递
package event

const (
	EventGroupCreated   = "group_created"
	EventGroupJoined    = "group_joined"
	EventGroupDismissed = "group_dismissed"
	EventGroupUpdated   = "group_updated"
	EventUserUpdated    = "user_updated"
	EventGroupApplyPassed = "group_apply_passed"
	EventFriendApplyPassed = "friend_apply_passed"
	EventFriendBlacked   = "friend_blacked"
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

// GroupUpdatedEvent 群信息变更事件（relation → message）
type GroupUpdatedEvent struct {
	GroupId string  `json:"group_id"`
	Name    *string `json:"name,omitempty"`
	Avatar  *string `json:"avatar,omitempty"`
}

// UserUpdatedEvent 用户资料变更事件（user → message）
type UserUpdatedEvent struct {
	UserId   string  `json:"user_id"`
	Nickname *string `json:"nickname,omitempty"`
	Avatar   *string `json:"avatar,omitempty"`
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
