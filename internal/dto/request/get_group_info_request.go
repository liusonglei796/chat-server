package request

type GetGroupInfoRequest struct {
	GroupId string `json:"group_id" form:"group_id" binding:"required"`
}
