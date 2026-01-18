package request

// DismissGroupRequest 解散群聊请求
// 使用位置:
//   - handler/group_handler.go: DismissGroupHandler
//
// 注意: OwnerId (操作者) 通过 JWT 获取，Service层校验权限
type DismissGroupRequest struct {
	GroupId string `json:"group_id" binding:"required"`
}
