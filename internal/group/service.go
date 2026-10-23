package group

import (
	"github.com/redis/go-redis/v9"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/pkg/idempotent"
)

// GroupService 群组业务逻辑实现
type GroupService struct {
	uow         UoW
	cache       cacheinfra.AsyncCacheService
	cacheHelper *cacheinfra.Helper
	guard       idempotent.Guard
}

// NewGroupService 构造函数，注入所有依赖
func NewGroupService(uow UoW, cacheService cacheinfra.AsyncCacheService, guard ...idempotent.Guard) *GroupService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	var helper *cacheinfra.Helper
	if cacheService != nil {
		helper = cacheinfra.NewHelper(cacheService)
	}
	return &GroupService{
		uow:         uow,
		cache:       cacheService,
		cacheHelper: helper,
		guard:       g,
	}
}
