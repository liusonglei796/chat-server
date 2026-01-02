package request

// GetContactInfoRequest 获取联系人信息请求
type GetContactInfoRequest struct {
	ContactId string `json:"contact_id" form:"contact_id" binding:"required"`
}
