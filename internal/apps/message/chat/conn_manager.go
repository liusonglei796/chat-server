package chat

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
)

// ShardCount 分片数量（采用 32 分片将全局锁竞争稀释到 1/32）
const ShardCount = 32

// ConnShard 单个分片：独立的读写锁与局部 map
type ConnShard struct {
	sync.RWMutex
	conns map[string]*UserConn
}

// ConnManager 工业级分段读写锁长连接管理器
// 1. 采用 32 分段锁降低并发写冲突
// 2. 读写分离：极高频的消息直推仅获取分片读锁 (RLock)，完全无锁阻塞
// 3. 在线人数统计基于 CPU 原子操作 (atomic.Int64)，O(1) 瞬间获取，杜绝遍历全表
type ConnManager struct {
	shards      [ShardCount]*ConnShard
	onlineCount atomic.Int64
}

// NewConnManager 创建 32 分片长连接管理器
func NewConnManager() *ConnManager {
	cm := &ConnManager{}
	for i := 0; i < ShardCount; i++ {
		cm.shards[i] = &ConnShard{
			conns: make(map[string]*UserConn),
		}
	}
	return cm
}

// getShard 基于 FNV-1a 哈希将用户 UID 均匀映射至分片
func (cm *ConnManager) getShard(userId string) *ConnShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(userId))
	return cm.shards[h.Sum32()%ShardCount]
}

// Get 根据用户 ID 获取长连接对象（加读锁，支持高并发并行读取）
func (cm *ConnManager) Get(userId string) *UserConn {
	shard := cm.getShard(userId)
	shard.RLock()
	defer shard.RUnlock()
	return shard.conns[userId]
}

// Store 注册/更新用户长连接（加独占写锁，更新原子计数）
func (cm *ConnManager) Store(userId string, conn *UserConn) {
	shard := cm.getShard(userId)
	shard.Lock()
	_, exists := shard.conns[userId]
	shard.conns[userId] = conn
	shard.Unlock()

	if !exists {
		cm.onlineCount.Add(1)
	}
}

// Delete 移除用户长连接（加独占写锁，更新原子计数）
func (cm *ConnManager) Delete(userId string) {
	shard := cm.getShard(userId)
	shard.Lock()
	_, exists := shard.conns[userId]
	if exists {
		delete(shard.conns, userId)
	}
	shard.Unlock()

	if exists {
		cm.onlineCount.Add(-1)
	}
}

// Load 读取连接并返回是否存在（兼顾原 sync.Map 习惯契约）
func (cm *ConnManager) Load(userId string) (*UserConn, bool) {
	conn := cm.Get(userId)
	return conn, conn != nil
}

// Count O(1) 纳秒级无锁获取当前单机在线长连接数
func (cm *ConnManager) Count() int64 {
	return cm.onlineCount.Load()
}

// Range 遍历所有分片中的连接（供广播或优雅退出使用）
func (cm *ConnManager) Range(f func(userId string, conn *UserConn) bool) {
	for i := 0; i < ShardCount; i++ {
		shard := cm.shards[i]
		shard.RLock()
		for uid, conn := range shard.conns {
			if !f(uid, conn) {
				shard.RUnlock()
				return
			}
		}
		shard.RUnlock()
	}
}
