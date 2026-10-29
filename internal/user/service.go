package user

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/idempotency"
	"kama_chat_server/pkg/idempotent"
)

// UserService 用户业务逻辑实现
type UserService struct {
	db          *gorm.DB
	store       Store
	cache       cacheinfra.AsyncCacheService
	cacheHelper *cacheinfra.Helper
	guard       idempotent.Guard
	idempStore  idempotency.Store
}

// NewUserService 构造函数，注入所有依赖
func NewUserService(db *gorm.DB, store Store, cacheService cacheinfra.AsyncCacheService, guard ...idempotent.Guard) *UserService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc != nil && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	var helper *cacheinfra.Helper
	if cacheService != nil {
		helper = cacheinfra.NewHelper(cacheService)
	}
	var idemp idempotency.Store
	if db != nil {
		idemp = idempotency.NewGormStore(db)
	}
	return &UserService{
		db:          db,
		store:       store,
		cache:       cacheService,
		cacheHelper: helper,
		guard:       g,
		idempStore:  idemp,
	}
}
