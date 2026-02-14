package group

// MuteMemberRequest 群成员禁言请求
type MuteMemberRequest struct {
	// GroupId 群组 UUID
	GroupId string `json:"group_id" binding:"required"`
	// UserId 被禁言的用户 UUID
	UserId string `json:"user_id" binding:"required"`
	// Duration 禁言时长（分钟），0 表示取消禁言
	Duration int `json:"duration" binding:"min=0"`
}
