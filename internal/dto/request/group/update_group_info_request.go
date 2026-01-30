package group

// UpdateGroupInfoRequest 更新群聊信息请求
// 使用位置:
//   - handler/group_handler.go: UpdateGroupInfoHandler
//
// 注意: 操作者ID 通过 JWT 获取，Service层校验权限
type UpdateGroupInfoRequest struct {
	Uuid    string `json:"uuid" binding:"required"`
	Name    string `json:"name"`
	Avatar  string `json:"avatar"`
	AddMode int8   `json:"add_mode"`
	Notice  string `json:"notice"`
}
