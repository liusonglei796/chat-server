package request

// OwnlistRequest 通用列表请求（当前用户ID）
type OwnlistRequest struct {
	UserId string `json:"user_id" form:"user_id" binding:"required"`
}
