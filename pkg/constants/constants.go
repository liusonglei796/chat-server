package constants

const (
	CHANNEL_SIZE               = 100              // 通道大小
	FILE_MAX_SIZE              = 30 << 20         // multipart 表单最大内存缓冲: 30 MB
	AVATAR_MAX_SIZE            = 5 << 20          // 头像文件最大: 5 MB
	UPLOAD_FILE_MAX_SIZE       = 30 << 20         // 普通上传文件最大: 30 MB
	REDIS_TIMEOUT              = 1                // redis timeout (分钟)
	REFRESH_TOKEN_EXPIRY_HOURS = 168              // Refresh Token 有效期（小时），168小时 = 7天
	AI_ASSISTANT_ID            = "U_AI_ASSISTANT" // AI 助手 ID
)
