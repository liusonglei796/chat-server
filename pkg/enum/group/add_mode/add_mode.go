package add_mode

// 群组加入方式
// DIRECT: 直接加入，无需审核
// AUDIT: 需要管理员审核后才能加入
const (
	DIRECT = iota
	AUDIT
)
