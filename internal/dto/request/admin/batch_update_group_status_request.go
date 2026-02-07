package admin

// BatchUpdateGroupStatusRequest 批量更新群组状态请求
// 使用位置: handler/admin_handler.go: SetGroupsStatus
//
// 说明: 仅管理员可调用，支持批量操作群组状态
//   - enable: 启用群组
//   - disable: 禁用群组
//   - delete: 删除群组
type BatchUpdateGroupStatusRequest struct {
	GroupUUIDs []string `json:"group_uuids" binding:"required,min=1"`                  // 群组ID列表
	Action     string   `json:"action" binding:"required,oneof=enable disable delete"` // 操作类型
}
