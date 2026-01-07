package redis

import (
	"encoding/json"
	"errors"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// cacheTask 定义缓存任务（纯闭包模式）
type cacheTask struct {
	Action func() // 要执行的操作
}

// cacheTaskChan 缓冲通道，用于接收缓存任务
var cacheTaskChan chan *cacheTask

// InitCacheWorker 初始化缓存 Worker Pool
// workerNum: 后台协程数量
// bufferSize: 通道缓冲区大小
func InitCacheWorker(workerNum int, bufferSize int) {
	cacheTaskChan = make(chan *cacheTask, bufferSize)

	for i := 0; i < workerNum; i++ {
		go startWorker()
	}
	zap.L().Info("Redis Cache Workers started", zap.Int("workers", workerNum), zap.Int("buffer", bufferSize))
}

// startWorker 启动单个 Worker 消费循环
func startWorker() {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error("Redis Worker panic", zap.Any("recover", r))
			go startWorker() // 重启
		}
	}()

	for task := range cacheTaskChan {
		if task.Action != nil {
			task.Action()
		}
	}
}

// SubmitCacheTask 提交异步缓存任务（通用入口）
// action: 要执行的操作闭包
// 使用示例:
//
//	myredis.SubmitCacheTask(func() {
//	    myredis.DelKeysWithPrefix("group_info_" + groupId)
//	})
func SubmitCacheTask(action func()) {
	select {
	case cacheTaskChan <- &cacheTask{Action: action}:
		// 成功放入
	default:
		// 降级：同步执行
		zap.L().Warn("Redis cache task channel full, executing synchronously")
		action()
	}
}

// UpdateUserChatCache 异步更新单聊缓存
func UpdateUserChatCache(message model.Message, rsp respond.GetMessageListRespond) {
	SubmitCacheTask(func() {
		key := "message_list_" + message.SendId + "_" + message.ReceiveId
		rspString, err := GetKeyNilIsErr(key)
		if err == nil {
			var list []respond.GetMessageListRespond
			if err := json.Unmarshal([]byte(rspString), &list); err == nil {
				list = append(list, rsp)
				if rspByte, err := json.Marshal(list); err == nil {
					SetKeyEx(key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
				}
			}
		} else if !errors.Is(err, redis.Nil) {
			zap.L().Error("Redis update user cache failed", zap.Error(err))
		}
	})
}

// UpdateGroupChatCache 异步更新群聊缓存
func UpdateGroupChatCache(message model.Message, rsp respond.GetGroupMessageListRespond) {
	SubmitCacheTask(func() {
		key := "group_messagelist_" + message.ReceiveId
		rspString, err := GetKeyNilIsErr(key)
		if err == nil {
			var list []respond.GetGroupMessageListRespond
			if err := json.Unmarshal([]byte(rspString), &list); err == nil {
				list = append(list, rsp)
				if rspByte, err := json.Marshal(list); err == nil {
					SetKeyEx(key, string(rspByte), time.Minute*constants.REDIS_TIMEOUT)
				}
			}
		} else if !errors.Is(err, redis.Nil) {
			zap.L().Error("Redis update group cache failed", zap.Error(err))
		}
	})
}
