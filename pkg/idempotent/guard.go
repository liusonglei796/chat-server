package idempotent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"kama_chat_server/pkg/errorx"
)

// Guard 工业级第三代状态机幂等保护接口 (Stateful Settlement Guard)
type Guard interface {
	// Execute 执行带状态机幂等保护的业务操作
	// key: 全局业务幂等键（例如 "grp:create:U1001:client-uuid"）
	// leaseTTL: 并发占锁短租约时长（防并发连击与 crash 死锁，推荐 3s~5s）
	// successTTL: 成功结果持久化保留时长（防网络超时重试回显，推荐 1h~24h）
	// fn: 业务执行闭包，返回原始结果字符串（如 ID 或 JSON 数据）
	Execute(ctx context.Context, key string, leaseTTL time.Duration, successTTL time.Duration, fn func(ctx context.Context) (string, error)) (result string, isReplay bool, err error)
}

// acquireLua 核心状态机抢占与回放原子脚本
// 返回数组: [code, result]
// code 取值:
//   - SUCCESS_REPLAY: 终态成功，直接回显 result
//   - IN_PROGRESS: 短租约仍在有效期内，拦截并发连击
//   - ACQUIRED: 正常抢占为 PROCESSING 状态
//   - LEASE_HEALED: 租约超时自愈，新请求接管执行
const acquireLua = `
local key = KEYS[1]
local lease_timeout = tonumber(ARGV[1])
local req_id = ARGV[2]

local status = redis.call("HGET", key, "status")
local result = redis.call("HGET", key, "result") or ""
local updated_at = tonumber(redis.call("HGET", key, "updated_at") or "0")

local time_arr = redis.call("TIME")
local current_time = tonumber(time_arr[1]) * 1000 + math.floor(tonumber(time_arr[2]) / 1000)

-- 1. 已成功 -> 幂等结果回显
if status == "SUCCESS" then
    return {"SUCCESS_REPLAY", result}
end

-- 2. 处理中 -> 检查短租约是否依然有效
if status == "PROCESSING" then
    if (current_time - updated_at) < lease_timeout then
        return {"IN_PROGRESS", ""}
    else
        -- 租约失效 -> 判定前序执行节点崩溃，租约自愈，允许新请求接管
        redis.call("HSET", key, "owner", req_id, "updated_at", current_time)
        return {"LEASE_HEALED", ""}
    end
end

-- 3. 初始或失败重入态 -> 正常抢占为 PROCESSING
redis.call("HSET", key, "status", "PROCESSING", "owner", req_id, "updated_at", current_time)
local temp_ttl = math.ceil(lease_timeout / 1000) * 3
if temp_ttl < 30 then
    temp_ttl = 30
end
redis.call("EXPIRE", key, temp_ttl)
return {"ACQUIRED", ""}
`

// successLua 固化成功终态脚本（含持有者 Fencing 检验，防过期覆写）
const successLua = `
local key = KEYS[1]
local result = ARGV[1]
local success_ttl = tonumber(ARGV[2])
local owner = ARGV[3]

local current_owner = redis.call("HGET", key, "owner")
if current_owner == owner then
    local time_arr = redis.call("TIME")
    local current_time = tonumber(time_arr[1]) * 1000 + math.floor(tonumber(time_arr[2]) / 1000)
    redis.call("HSET", key, "status", "SUCCESS", "result", result, "updated_at", current_time)
    redis.call("EXPIRE", key, success_ttl)
    return "OK"
end
return "STALE_OWNER"
`

// failLua 业务失败清理脚本（仅当持有者一致时清理锁，允许下次重试）
const failLua = `
local key = KEYS[1]
local owner = ARGV[1]

local current_owner = redis.call("HGET", key, "owner")
if current_owner == owner then
    redis.call("DEL", key)
    return "OK"
end
return "STALE_OWNER"
`

// RedisGuard 基于 Redis Lua 状态机的第三代幂等守卫实现
type RedisGuard struct {
	client *redis.Client
}

// NewRedisGuard 创建 Redis 幂等守卫
func NewRedisGuard(client *redis.Client) *RedisGuard {
	return &RedisGuard{client: client}
}

// Execute 执行带状态机幂等保护的业务操作
// 严格按照业务执行的时间先后顺序编排调用链路
func (g *RedisGuard) Execute(ctx context.Context, key string, leaseTTL time.Duration, successTTL time.Duration, fn func(ctx context.Context) (string, error)) (result string, isReplay bool, err error) {
	// Step 1.1: 校验入参及基础依赖
	// 来源: 调用方传入的 key 与 redis client
	// 用于: 判断是否跳过幂等检查或降级直连
	if key == "" || g.client == nil {
		res, execErr := fn(ctx)
		return res, false, execErr
	}

	// 统一生成带命名空间的真实 Redis Key
	redisKey := "idemp:st:" + key
	// 生成当前执行周期的唯一所有者令牌 (NanoID)
	ownerID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	leaseMs := int(leaseTTL.Milliseconds())
	if leaseMs <= 0 {
		leaseMs = 5000 // 默认 5 秒短租约
	}

	// Step 1.2: 执行 Lua 脚本原子探针（抢占短租约 / 历史回放探针）
	// 来源: redisKey, leaseMs, ownerID
	// 用于: Step 1.3 决定是回放、拦截还是继续执行
	probeRes, probeErr := g.probeOrAcquireLease(ctx, redisKey, leaseMs, ownerID)
	if probeErr != nil {
		// Redis 故障降级处理：记录警报日志并放行执行底层业务，保障可用性
		zap.L().Warn("幂等中间件 Redis 探针故障，降级放行业务", zap.String("key", redisKey), zap.Error(probeErr))
		res, execErr := fn(ctx)
		return res, false, execErr
	}

	// Step 1.3: 分支状态处理
	// 来源: Step 1.2 返回的状态码 probeRes.Code
	// 用于: 执行回放或拦截
	if probeRes.Code == "SUCCESS_REPLAY" {
		// 命中幂等回显：0ms 秒级返回已有结果，绝不再次执行业务
		return probeRes.Result, true, nil
	}
	if probeRes.Code == "IN_PROGRESS" {
		// 命中并发连击：阻断重复提交
		return "", false, errorx.New(errorx.CodeTooManyRequests, "请求正在处理中，请勿重复提交")
	}

	// Step 1.4: 获得执行权，调用业务闭包执行真实逻辑
	// 来源: Step 1.2 成功获取的 ACQUIRED 或 LEASE_HEALED 租约
	// 用于: Step 1.5 根据执行结果决定持久化终态还是解冻
	res, bizErr := fn(ctx)
	if bizErr != nil {
		// 业务失败：异步清除占用，解冻状态以支持重试
		_ = g.cleanupOnFailure(context.Background(), redisKey, ownerID)
		return "", false, bizErr
	}

	// Step 1.5: 业务确定性成功，持久化终态并顺延生命周期
	// 来源: Step 1.4 返回的成功结果 res
	// 用于: 支撑后续网络重试时直接 0ms 幂等回显
	successSec := int(successTTL.Seconds())
	if successSec <= 0 {
		successSec = 86400 // 默认缓存 24 小时
	}
	_ = g.persistSuccessState(context.Background(), redisKey, res, successSec, ownerID)

	return res, false, nil
}

// probeResult Lua 探针返回结构
type probeResult struct {
	Code   string
	Result string
}

// probeOrAcquireLease 探针具体调用
// 来源: Step 1.2 主流程
// 用于: 执行 acquireLua 脚本
func (g *RedisGuard) probeOrAcquireLease(ctx context.Context, key string, leaseMs int, ownerID string) (*probeResult, error) {
	val, err := g.client.Eval(ctx, acquireLua, []string{key}, leaseMs, ownerID).Slice()
	if err != nil {
		return nil, err
	}
	if len(val) < 2 {
		return nil, errors.New("invalid lua script response")
	}
	return &probeResult{
		Code:   fmt.Sprintf("%v", val[0]),
		Result: fmt.Sprintf("%v", val[1]),
	}, nil
}

// cleanupOnFailure 业务执行异常时的清理流程
// 来源: Step 1.4 业务异常分支
// 用于: 删除正在处理中的临时占位
func (g *RedisGuard) cleanupOnFailure(ctx context.Context, key, ownerID string) error {
	return g.client.Eval(ctx, failLua, []string{key}, ownerID).Err()
}

// persistSuccessState 业务执行成功时的持久化流程
// 来源: Step 1.5 业务成功分支
// 用于: 将真实返回值与 SUCCESS 状态持久化
func (g *RedisGuard) persistSuccessState(ctx context.Context, key, result string, successTTL int, ownerID string) error {
	return g.client.Eval(ctx, successLua, []string{key}, result, successTTL, ownerID).Err()
}

// ExecuteJSON 泛型封装：自动序列化与反序列化复杂业务结构体
// 来源: 业务上层调用
// 用于: 提供强类型友好的幂等调用体验
func ExecuteJSON[T any](ctx context.Context, g Guard, key string, leaseTTL, successTTL time.Duration, fn func(ctx context.Context) (T, error)) (T, bool, error) {
	var zero T
	if g == nil || key == "" {
		res, err := fn(ctx)
		return res, false, err
	}

	strRes, isReplay, err := g.Execute(ctx, key, leaseTTL, successTTL, func(c context.Context) (string, error) {
		data, bizErr := fn(c)
		if bizErr != nil {
			return "", bizErr
		}
		bytes, marshalErr := json.Marshal(data)
		if marshalErr != nil {
			return "", marshalErr
		}
		return string(bytes), nil
	})

	if err != nil {
		return zero, false, err
	}

	var target T
	if unmarshalErr := json.Unmarshal([]byte(strRes), &target); unmarshalErr != nil {
		return zero, false, unmarshalErr
	}
	return target, isReplay, nil
}
