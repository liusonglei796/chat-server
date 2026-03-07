package snowflake

import (
	"sync"

	"kama_chat_server/internal/config"

	"github.com/bwmarrin/snowflake"
	"go.uber.org/zap"
)

// getNode 惰性初始化雪花算法节点，线程安全，仅执行一次
var getNode = sync.OnceValue(func() *snowflake.Node {
	machineID := config.GetConfig().SnowflakeConfig.MachineID
	if machineID < 0 || machineID > 1023 {
		machineID = 1 // 默认节点 ID
		zap.L().Warn("Invalid MachineID in config, using default value 1")
	}
	node, err := snowflake.NewNode(machineID)
	if err != nil {
		zap.L().Fatal("Failed to initialize snowflake node", zap.Error(err))
	}
	zap.L().Info("Snowflake node initialized", zap.Int64("machineID", machineID))
	return node
})

// GenerateIDString 生成雪花 ID (string)
// 用于 JSON 序列化，避免 JavaScript 精度丢失
func GenerateIDString() string {
	return getNode().Generate().String()
}
