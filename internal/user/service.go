package user

import (
	"regexp"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/pkg/idempotent"
)

// UserService 用户业务逻辑实现
type UserService struct {
	uow         UoW
	cache       cacheinfra.AsyncCacheService
	cacheHelper *cacheinfra.Helper
	guard       idempotent.Guard
}

// NewUserService 构造函数，注入所有依赖
func NewUserService(uow UoW, cacheService cacheinfra.AsyncCacheService, guard ...idempotent.Guard) *UserService {
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
	return &UserService{
		uow:         uow,
		cache:       cacheService,
		cacheHelper: helper,
		guard:       g,
	}
}

// checkTelephoneValid 检验电话是否有效
func (u *UserService) checkTelephoneValid(telephone string) bool {
	pattern := `^1([38][0-9]|14[579]|5[^4]|16[6]|7[1-35-8]|9[189])\d{8}$`
	match, err := regexp.MatchString(pattern, telephone)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
	return match
}

// checkEmailValid 校验邮箱是否有效
func (u *UserService) checkEmailValid(email string) bool {
	pattern := `^[^\s@]+@[^\s@]+\.[^\s@]+$`
	match, err := regexp.MatchString(pattern, email)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
	}
	return match
}
