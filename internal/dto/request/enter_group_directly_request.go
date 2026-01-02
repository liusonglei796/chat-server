package request

// EnterGroupDirectlyRequest 直接加入群组请求
type EnterGroupDirectlyRequest struct {
	UserId  string `json:"user_id" binding:"required"`
	GroupId string `json:"group_id" binding:"required"`
}
