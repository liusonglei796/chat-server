package request

type GetUserInfoRequest struct {
	Uuid string `json:"uuid" form:"uuid" binding:"required"`
}
