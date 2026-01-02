package request

type DeleteGroupsRequest struct {
	UuidList []string `json:"uuidList" binding:"required"`
}
