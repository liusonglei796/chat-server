package snowflake

import (
	"sync"

	"github.com/bwmarrin/snowflake"
	"go.uber.org/zap"

	"kama_chat_server/internal/config"
)

var (
	node     *snowflake.Node
	nodeOnce sync.Once
)

// Init 初始化雪花算法节点
// 应在程序启动时调用一次
func Init() {
	nodeOnce.Do(func() {
		machineID := config.GetConfig().SnowflakeConfig.MachineID
		if machineID < 0 || machineID > 1023 {
			machineID = 1 // 默认节点 ID
			zap.L().Warn("Invalid MachineID in config, using default value 1")
		}
		var err error
		node, err = snowflake.NewNode(machineID)
		if err != nil {
			zap.L().Fatal("Failed to initialize snowflake node", zap.Error(err))
		}
		zap.L().Info("Snowflake node initialized", zap.Int64("machineID", machineID))
	})
}

// GenerateID 生成雪花 ID (int64)
func GenerateID() int64 {
	if node == nil {
		Init()
	}
	return node.Generate().Int64()
}

// GenerateIDString 生成雪花 ID (string)
// 用于 JSON 序列化，避免 JavaScript 精度丢失
func GenerateIDString() string {
	if node == nil {
		Init()
	}
	return node.Generate().String()
}
