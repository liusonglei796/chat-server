package store

import "fmt"

// TxExecutor 事务执行能力的最小契约
// 各服务的 UoW 接口自行内嵌/声明该能力，并组合本服务自己的 Store 访问器，
// 从而在编译期限制服务只能访问属于自己的表。
type TxExecutor interface {
	WithTx(fn func(tx any) error) error
}

// WithTx 泛型事务执行器
// 将 WithTx 回调参数从 any 断言为服务自定义的 UoW 接口（如 group.groupUoW），
// 使回调体内能类型安全地访问本服务自己的 Store。
// 断言失败视为程序 bug，直接返回错误。
func WithTx[T any](exec TxExecutor, fn func(tx T) error) error {
	return exec.WithTx(func(tx any) error {
		t, ok := tx.(T)
		if !ok {
			return fmt.Errorf("transaction callback type mismatch: got %T, want %T", tx, *new(T))
		}
		return fn(t)
	})
}
