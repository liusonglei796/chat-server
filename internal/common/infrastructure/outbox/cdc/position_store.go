package cdc

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/redis/go-redis/v9"
)

// PositionStore 定义 Binlog 位点存储接口
type PositionStore interface {
	// Get 读取已持久化的位点，若不存在返回空位点
	Get(ctx context.Context) (mysql.Position, error)
	// Save 持久化保存位点
	Save(ctx context.Context, pos mysql.Position) error
}

const (
	defaultBinlogFileKey = "cdc:outbox:binlog_file"
	defaultBinlogPosKey  = "cdc:outbox:binlog_pos"
)

// RedisPositionStore 基于 Redis 实现的 Binlog 位点持久化存储
type RedisPositionStore struct {
	client  redis.UniversalClient
	fileKey string
	posKey  string
}

// NewRedisPositionStore 创建基于 Redis 的位点存储器
func NewRedisPositionStore(client redis.UniversalClient) *RedisPositionStore {
	return &RedisPositionStore{
		client:  client,
		fileKey: defaultBinlogFileKey,
		posKey:  defaultBinlogPosKey,
	}
}

// Get 读取已记录的 Binlog 位点
func (s *RedisPositionStore) Get(ctx context.Context) (mysql.Position, error) {
	pipe := s.client.Pipeline()
	fileCmd := pipe.Get(ctx, s.fileKey)
	posCmd := pipe.Get(ctx, s.posKey)
	_, err := pipe.Exec(ctx)
	if err != nil {
		if err == redis.Nil {
			return mysql.Position{}, nil
		}
		return mysql.Position{}, fmt.Errorf("read binlog position from redis failed: %w", err)
	}

	fileName := fileCmd.Val()
	posStr := posCmd.Val()
	if fileName == "" || posStr == "" {
		return mysql.Position{}, nil
	}

	posVal, err := strconv.ParseUint(posStr, 10, 32)
	if err != nil {
		return mysql.Position{}, fmt.Errorf("invalid binlog pos value in redis: %w", err)
	}

	return mysql.Position{
		Name: fileName,
		Pos:  uint32(posVal),
	}, nil
}

// Save 将位点存入 Redis
func (s *RedisPositionStore) Save(ctx context.Context, pos mysql.Position) error {
	if pos.Name == "" {
		return nil
	}
	pipe := s.client.Pipeline()
	pipe.Set(ctx, s.fileKey, pos.Name, 0)
	pipe.Set(ctx, s.posKey, strconv.FormatUint(uint64(pos.Pos), 10), 0)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("save binlog position to redis failed: %w", err)
	}
	return nil
}

// MemoryPositionStore 内存型位点存储器，用于单测
type MemoryPositionStore struct {
	pos mysql.Position
}

// NewMemoryPositionStore 创建内存型位点存储器
func NewMemoryPositionStore() *MemoryPositionStore {
	return &MemoryPositionStore{}
}

// Get 读取内存位点
func (m *MemoryPositionStore) Get(_ context.Context) (mysql.Position, error) {
	return m.pos, nil
}

// Save 保存位点到内存
func (m *MemoryPositionStore) Save(_ context.Context, pos mysql.Position) error {
	m.pos = pos
	return nil
}
