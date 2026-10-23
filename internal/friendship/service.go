package friendship

import (
	cacheinfra "kama_chat_server/internal/infra/cache"
)

// FriendshipService 好友关系业务逻辑实现
type FriendshipService struct {
	uow   UoW
	cache cacheinfra.AsyncCacheService
}

// NewFriendshipService 构造函数，注入所有依赖
func NewFriendshipService(uow UoW, cacheService ...cacheinfra.AsyncCacheService) *FriendshipService {
	var c cacheinfra.AsyncCacheService
	if len(cacheService) > 0 {
		c = cacheService[0]
	}
	return &FriendshipService{
		uow:   uow,
		cache: c,
	}
}
