// Package event 定义跨服务领域事件 DTO 与类型常量
// 事件经 outbox → Kafka domain_events 在服务间传递
package event

const (
	EventGroupCreated   = "group_created"
	EventGroupJoined    = "group_joined"
	EventGroupDismissed = "group_dismissed"
	EventGroupUpdated   = "group_updated"
	EventUserUpdated    = "user_updated"
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
