package request

type CheckGroupAddModeRequest struct {
	GroupId string `json:"group_id" form:"group_id" binding:"required"`
}
