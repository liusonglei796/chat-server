package request

type SetGroupsStatusRequest struct {
	UuidList []string `json:"uuid_list" binding:"required,min=1"`
	Status   int8     `json:"status"`
}
