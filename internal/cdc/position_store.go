package cdc

import (
	"context"
	"fmt"

	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/redis/go-redis/v9"
)

// GTIDStore 定义 Binlog GTID 位点存储接口
type GTIDStore interface {
	// Get 读取已持久化的 GTID 集合，若不存在返回 nil
	Get(ctx context.Context) (mysql.GTIDSet, error)
	// Save 持久化保存 GTID 集合
	Save(ctx context.Context, set mysql.GTIDSet) error
}

const (
	defaultGTIDKey = "cdc:outbox:gtid"
)

// RedisGTIDStore 基于 Redis 实现的 GTID 持久化存储
type RedisGTIDStore struct {
	client  redis.UniversalClient
	gtidKey string
	flavor  string
}

// NewRedisGTIDStore 创建基于 Redis 的 GTID 存储器
func NewRedisGTIDStore(client redis.UniversalClient) *RedisGTIDStore {
	return &RedisGTIDStore{
		client:  client,
		gtidKey: defaultGTIDKey,
		flavor:  "mysql",
	}
}

// Get 读取已记录的 GTID 集合
func (s *RedisGTIDStore) Get(ctx context.Context) (mysql.GTIDSet, error) {
	val, err := s.client.Get(ctx, s.gtidKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("read gtid from redis failed: %w", err)
	}

	if val == "" {
		return nil, nil
	}

	gtidSet, err := mysql.ParseGTIDSet(s.flavor, val)
	if err != nil {
		return nil, fmt.Errorf("invalid gtid set in redis (%s): %w", val, err)
	}

	return gtidSet, nil
}

// Save 将 GTID 集合存入 Redis
func (s *RedisGTIDStore) Save(ctx context.Context, set mysql.GTIDSet) error {
	if set == nil || set.IsEmpty() {
		return nil
	}
	err := s.client.Set(ctx, s.gtidKey, set.String(), 0).Err()
	if err != nil {
		return fmt.Errorf("save gtid set to redis failed: %w", err)
	}
	return nil
}

// MemoryGTIDStore 内存型 GTID 存储器，用于单测
type MemoryGTIDStore struct {
	set mysql.GTIDSet
}

// NewMemoryGTIDStore 创建内存型 GTID 存储器
func NewMemoryGTIDStore() *MemoryGTIDStore {
	return &MemoryGTIDStore{}
}

// Get 读取内存 GTID 集合
func (m *MemoryGTIDStore) Get(_ context.Context) (mysql.GTIDSet, error) {
	if m.set == nil {
		return nil, nil
	}
	return m.set.Clone(), nil
}

// Save 保存 GTID 集合到内存
func (m *MemoryGTIDStore) Save(_ context.Context, set mysql.GTIDSet) error {
	if set != nil {
		m.set = set.Clone()
	}
	return nil
}
