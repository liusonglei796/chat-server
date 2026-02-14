package friendship

// BatchDeleteRequest 批量删除请求（通用）
// 统一使用 UuidList，支持单个或批量删除（min=1）
type BatchDeleteRequest struct {
	UuidList []string `json:"uuid_list" binding:"required,min=1"`
}
