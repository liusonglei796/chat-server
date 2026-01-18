package request

// CreateGroupRequest 创建群聊请求
// 使用位置:
//   - handler/group_handler.go: CreateGroupHandler
//
// 注意: OwnerId (群主) 通过 JWT 获取
type CreateGroupRequest struct {
	Name    string `json:"name" binding:"required"`
	Notice  string `json:"notice"`
	AddMode int8   `json:"add_mode"`
	Avatar  string `json:"avatar"`
}
