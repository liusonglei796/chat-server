package apply

import (
	"github.com/redis/go-redis/v9"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/pkg/idempotent"
)

// ApplyService 申请业务逻辑实现
// 设计原则说明：
//  1. 读操作（如获取申请列表）：采用【直接查询数据库】模式。
//     原因：申请/审批流程对数据一致性要求极高（防脏读、防重复），且相比消息/联系列表，其访问频率较低，直查DB可确保逻辑安全且性能可控。
//  2. 写操作（如发起/通过申请）：采用【写库成功后删除缓存】模式。
//     原因：作为数据生产者，在变更状态后主动失效下游服务（如ContactService）的缓存，保证系统数据的最终一致性。
type ApplyService struct {
	uow         UoW
	cache       cacheinfra.AsyncCacheService
	cacheHelper *cacheinfra.Helper
	guard       idempotent.Guard
}

// NewApplyService 构造函数
func NewApplyService(uow UoW, cacheService cacheinfra.AsyncCacheService, guard ...idempotent.Guard) *ApplyService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	return &ApplyService{
		uow:         uow,
		cache:       cacheService,
		cacheHelper: cacheinfra.NewHelper(cacheService),
		guard:       g,
	}
}
