package message

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	bwsnowflake "github.com/bwmarrin/snowflake"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/common/config"
	"kama_chat_server/internal/common/domain/store"
	messagereq "kama_chat_server/internal/common/dto/request/message"
	messagersp "kama_chat_server/internal/common/dto/respond/message"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/infrastructure/metrics"
	"kama_chat_server/internal/common/infrastructure/snowflake"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/message/message_status"
	msgtype "kama_chat_server/pkg/enum/message/message_type"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
)

// MessageService 消息业务逻辑实现
// 通过构造函数注入 Store 和 Cache 依赖，遵循依赖倒置原则
type MessageService struct {
	messageStore store.MessageStore
	sessionStore store.SessionStore
	cache        store.AsyncCacheService
	// pushRecallNotify 撤回通知回调（可由下游单管道推送实现）
	pushRecallNotify func(messageUuid, receiveId string)
}

// NewMessageService 构造函数，注入所有依赖
func NewMessageService(
	messageStore store.MessageStore,
	sessionStore store.SessionStore,
	cacheService store.AsyncCacheService,
) *MessageService {
	m := &MessageService{
		messageStore: messageStore,
		sessionStore: sessionStore,
		cache:        cacheService,
	}
	m.pushRecallNotify = m.PushRecallNotify
	return m
}

// SetPushRecallNotify 注入撤回通知回调
func (m *MessageService) SetPushRecallNotify(notify func(messageUuid, receiveId string)) {
	m.pushRecallNotify = notify
}

// GetMessageList 获取聊天记录（分页）
func (m *MessageService) GetMessageList(ctx context.Context, requesterId, partnerId string, page, pageSize int) ([]messagersp.GetMessageListRespond, int64, error) {
	// 参数校验
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 权限校验: 必须是好友关系才能查看聊天记录
	fsStatus, err := grpc_client.CheckFriendshipStatus(ctx, requesterId, partnerId)
	if err != nil {
		zap.L().Error("check friendship via grpc error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}
	if fsStatus != 1 {
		return nil, 0, errorx.New(errorx.CodeForbidden, "你们不是好友，无法查看聊天记录")
	}

	userOneId := requesterId

	// 确保 ID 顺序一致，保证查询结果稳定
	if userOneId > partnerId {
		userOneId, partnerId = partnerId, userOneId
	}

	// 查数据库（带分页）
	messageList, total, err := m.messageStore.FindByUserIdsPaged(ctx, userOneId, partnerId, page, pageSize)
	if err != nil {
		zap.L().Error("find messages by user ids error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	return m.hydrateMessages(ctx, messageList), total, nil
}

// GetGroupMessageList 获取群聊消息记录（分页）
func (m *MessageService) GetGroupMessageList(ctx context.Context, userId, groupId string, page, pageSize int) ([]messagersp.GetMessageListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 权限校验: 只要有 Session 记录(未删除)即可查看历史消息，不仅仅是当前成员
	// 这样可以支持"退群后查看历史消息"的需求
	_, err := m.sessionStore.FindBySendIdAndReceiveId(ctx, userId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, 0, errorx.New(errorx.CodeForbidden, "您没有该群的会话记录")
		}
		zap.L().Error("Find session error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	// 分页查询数据库
	messageList, total, err := m.messageStore.FindByGroupIdPaged(ctx, groupId, page, pageSize)
	if err != nil {
		zap.L().Error("find group messages error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	return m.hydrateMessages(ctx, messageList), total, nil
}

// ResolveSessionId 归一化会话唯一标识：
// 1. 群聊：统一为 groupId（G开头）
// 2. 单聊：按字典序组合双方 UUID 为 P_{min}_{max}，保证双向查询与存储完全一致
func ResolveSessionId(sendId, receiveId string) string {
	if strings.HasPrefix(receiveId, "G") {
		return receiveId
	}
	if sendId > receiveId {
		sendId, receiveId = receiveId, sendId
	}
	return fmt.Sprintf("P_%s_%s", sendId, receiveId)
}

// snowflakeUuidToScore 将消息 UUID 或游标转换为 Redis ZSET score（微秒安全精度）
func snowflakeUuidToScore(uuid string) float64 {
	if uuid == "" {
		return 0
	}
	if strings.HasPrefix(uuid, "M") {
		raw := strings.TrimPrefix(uuid, "M")
		if sfId, err := bwsnowflake.ParseString(raw); err == nil {
			return float64(sfId.Time())*1000 + float64(int64(sfId)&0xfff)
		}
	}
	if ts, err := strconv.ParseInt(uuid, 10, 64); err == nil {
		if ts < 1e11 {
			return float64(ts * 1000000)
		} else if ts < 1e14 {
			return float64(ts * 1000)
		}
		return float64(ts)
	}
	return 0
}

func resolveNextCursor(messages []model.Message) string {
	if len(messages) == 0 {
		return ""
	}
	lastMsg := messages[len(messages)-1]
	if lastMsg.Uuid != "" {
		return lastMsg.Uuid
	}
	return strconv.FormatInt(lastMsg.CreatedAt.Unix(), 10)
}

func (m *MessageService) cacheMessageToZSet(message *model.Message) {
	if m.cache == nil || message.SessionId == "" {
		return
	}
	msgCopy := *message
	m.cache.SubmitTask(func() {
		key := constants.CacheKeyMessageZSet + msgCopy.SessionId
		score := snowflakeUuidToScore(msgCopy.Uuid)
		if score <= 0 {
			score = float64(msgCopy.CreatedAt.UnixMicro())
		}
		if score <= 0 {
			score = float64(time.Now().UnixMicro())
		}
		data, err := json.Marshal(msgCopy)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = m.cache.ZAdd(ctx, key, score, string(data))
		_ = m.cache.ZRemRangeByRank(ctx, key, 0, -201)
		_ = m.cache.Expire(ctx, key, 7*24*time.Hour)
	})
}

func (m *MessageService) updateRecalledMessageInZSet(sessionId, messageUuid string) {
	if m.cache == nil || sessionId == "" || messageUuid == "" {
		return
	}
	m.cache.SubmitTask(func() {
		key := constants.CacheKeyMessageZSet + sessionId
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		members, err := m.cache.ZRevRangeByScore(ctx, key, "+inf", "-inf", 0, 200)
		if err != nil || len(members) == 0 {
			return
		}
		for _, member := range members {
			var msg model.Message
			if err := json.Unmarshal([]byte(member), &msg); err == nil && msg.Uuid == messageUuid {
				_ = m.cache.ZRem(ctx, key, member)
				msg.Content = ""
				msg.Type = int8(msgtype.Recall)
				score := snowflakeUuidToScore(msg.Uuid)
				if updatedData, err := json.Marshal(msg); err == nil {
					_ = m.cache.ZAdd(ctx, key, score, string(updatedData))
				}
				break
			}
		}
	})
}

// getMessagesBySessionIdCursorWithCache 统一使用 session_id 游标查询，优先走 Redis ZSET，未命中/触底时走 MySQL
func (m *MessageService) getMessagesBySessionIdCursorWithCache(ctx context.Context, sessionId, cursor string, pageSize int) ([]messagersp.GetMessageListRespond, string, bool, error) {
	key := constants.CacheKeyMessageZSet + sessionId

	// 1. 尝试从 Redis ZSET 缓存读取
	if m.cache != nil {
		maxScore := "+inf"
		if cursor != "" {
			if score := snowflakeUuidToScore(cursor); score > 0 {
				maxScore = fmt.Sprintf("(%f", score)
			}
		}

		// 查询 pageSize + 1 条用于探测 hasMore
		members, err := m.cache.ZRevRangeByScore(ctx, key, maxScore, "-inf", 0, int64(pageSize+1))
		if err == nil && len(members) > 0 {
			var cachedMessages []model.Message
			for _, item := range members {
				var msg model.Message
				if err := json.Unmarshal([]byte(item), &msg); err == nil {
					cachedMessages = append(cachedMessages, msg)
				}
			}

			if len(cachedMessages) > 0 {
				hasMore := len(cachedMessages) > pageSize
				if hasMore {
					cachedMessages = cachedMessages[:pageSize]
					nextCursor := resolveNextCursor(cachedMessages)
					return m.hydrateMessages(ctx, cachedMessages), nextCursor, true, nil
				}

				// 返回条数 <= pageSize，检查是否到达会话最早消息
				totalInCache, cardErr := m.cache.ZCard(ctx, key)
				if cardErr == nil && totalInCache < 200 {
					return m.hydrateMessages(ctx, cachedMessages), "", false, nil
				}
				// 若 totalInCache >= 200，说明滑窗可能被截断，继续下沉 MySQL
			}
		}
	}

	// 2. 缓存未命中或滑窗触底：下沉查 MySQL (统一 session_id 游标点查，命中联合索引)
	result, err := m.messageStore.FindBySessionIdCursor(ctx, sessionId, cursor, pageSize)
	if err != nil {
		zap.L().Error("find messages by session id cursor error", zap.String("sessionId", sessionId), zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	// 3. 首页查询 (cursor == "") 且之前缓存未满时，异步回填最新消息到 ZSET
	if m.cache != nil && cursor == "" && len(result.Messages) > 0 {
		messagesToCache := make([]model.Message, len(result.Messages))
		copy(messagesToCache, result.Messages)
		m.cache.SubmitTask(func() {
			for _, msg := range messagesToCache {
				score := snowflakeUuidToScore(msg.Uuid)
				if score <= 0 {
					score = float64(msg.CreatedAt.UnixMicro())
				}
				if data, err := json.Marshal(msg); err == nil {
					_ = m.cache.ZAdd(context.Background(), key, score, string(data))
				}
			}
			_ = m.cache.ZRemRangeByRank(context.Background(), key, 0, -201)
			_ = m.cache.Expire(context.Background(), key, 7*24*time.Hour)
		})
	}

	return m.hydrateMessages(ctx, result.Messages), result.NextCursor, result.HasMore, nil
}

// GetMessageListCursor 获取两个用户之间的聊天记录（游标分页，统一 session_id + ZSET 缓存）
func (m *MessageService) GetMessageListCursor(ctx context.Context, requesterId, partnerId, cursor string, pageSize int) ([]messagersp.GetMessageListRespond, string, bool, error) {
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 权限校验: 必须是好友关系才能查看聊天记录
	fsStatus, err := grpc_client.CheckFriendshipStatus(ctx, requesterId, partnerId)
	if err != nil {
		zap.L().Error("check friendship via grpc error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}
	if fsStatus != 1 {
		return nil, "", false, errorx.New(errorx.CodeForbidden, "你们不是好友，无法查看聊天记录")
	}

	sessionId := ResolveSessionId(requesterId, partnerId)
	return m.getMessagesBySessionIdCursorWithCache(ctx, sessionId, cursor, pageSize)
}

// GetGroupMessageListCursor 获取群聊消息记录（游标分页，统一 session_id + ZSET 缓存）
func (m *MessageService) GetGroupMessageListCursor(ctx context.Context, userId, groupId, cursor string, pageSize int) ([]messagersp.GetMessageListRespond, string, bool, error) {
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 权限校验: 只要有 Session 记录(未删除)即可查看历史消息
	_, err := m.sessionStore.FindBySendIdAndReceiveId(ctx, userId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, "", false, errorx.New(errorx.CodeForbidden, "您没有该群的会话记录")
		}
		zap.L().Error("Find session error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	sessionId := groupId
	return m.getMessagesBySessionIdCursorWithCache(ctx, sessionId, cursor, pageSize)
}

// UploadAvatar 上传头像
func (m *MessageService) UploadAvatar(ctx context.Context, c *gin.Context) (string, error) {
	if err := c.Request.ParseMultipartForm(constants.FILE_MAX_SIZE); err != nil {
		zap.L().Error("parse multipart form error", zap.Error(err))
		return "", errorx.New(errorx.CodeInvalidParam, "文件过大，请上传小于 30MB 的文件")
	}
	mForm := c.Request.MultipartForm
	if len(mForm.File) == 0 {
		return "", errorx.New(errorx.CodeInvalidParam, "no file uploaded")
	}

	// 遍历所有文件，但既然是上传头像，通常只取第一个
	for _, headers := range mForm.File {
		for _, fileHeader := range headers {
			// 头像大小校验（5 MB）
			if fileHeader.Size > constants.AVATAR_MAX_SIZE {
				return "", errorx.New(errorx.CodeInvalidParam, "头像文件过大，最大支持 5MB")
			}
			// 限制为图片类型的 MIME
			filename, err := m.saveFile(fileHeader, config.GetConfig().StaticAvatarPath, "image/jpeg", "image/png", "image/gif")
			if err != nil {
				zap.L().Error("save avatar error", zap.Error(err))
				// 如果是参数错误（如文件类型不对），尝试处理下一个文件
				if errorx.GetCode(err) == errorx.CodeInvalidParam {
					continue
				}
				return "", errorx.ErrServerBusy
			}
			zap.L().Info("upload avatar success", zap.String("filename", filename))
			return filename, nil
		}
	}
	return "", errorx.New(errorx.CodeInvalidParam, "no file found")
}

// UploadFile 上传文件
func (m *MessageService) UploadFile(ctx context.Context, c *gin.Context) ([]string, error) {
	if err := c.Request.ParseMultipartForm(constants.FILE_MAX_SIZE); err != nil {
		zap.L().Error("parse multipart form error", zap.Error(err))
		return nil, errorx.New(errorx.CodeInvalidParam, "文件过大，请上传小于 30MB 的文件")
	}

	var uploadedFiles []string
	dstDir := config.GetConfig().StaticFilePath
	mForm := c.Request.MultipartForm

	for _, headers := range mForm.File {
		for _, fileHeader := range headers {
			// 单文件大小校验（30 MB）
			if fileHeader.Size > constants.UPLOAD_FILE_MAX_SIZE {
				// 回滚已上传的文件
				for _, f := range uploadedFiles {
					_ = os.Remove(filepath.Join(dstDir, f))
				}
				return nil, errorx.New(errorx.CodeInvalidParam, "单个文件过大，最大支持 30MB")
			}

			// 上传普通文件不限制 MIME，或者可以根据需求添加限制
			filename, err := m.saveFile(fileHeader, dstDir)
			if err != nil {
				zap.L().Error("save file error", zap.Error(err))

				// 发生错误，回滚已上传的文件，保证原子性
				for _, f := range uploadedFiles {
					_ = os.Remove(filepath.Join(dstDir, f))
				}

				return nil, errorx.ErrServerBusy
			}

			zap.L().Info("upload file success", zap.String("filename", filename), zap.Int64("size", fileHeader.Size))
			uploadedFiles = append(uploadedFiles, filename)
		}
	}

	return uploadedFiles, nil
}

// saveFile 通用保存文件方法，支持 Magic Bytes 类型校验
// fileHeader: 上传的文件头信息
// dstDir: 目标保存目录
// allowedMimes: 允许的 MIME 类型列表（可变参数，为空则不校验）
// 返回: 生成的新文件名, 错误
func (m *MessageService) saveFile(fileHeader *multipart.FileHeader, dstDir string, allowedMimes ...string) (string, error) {
	// 打开上传的文件，获取文件读取流
	src, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer src.Close() // 确保函数结束时关闭文件

	// 1. 读取前 512 字节进行 Magic Bytes 校验
	// Magic Bytes 是文件开头的特征字节，用于识别真实文件类型（防止伪造扩展名）
	buffer := make([]byte, 512)
	if _, err := src.Read(buffer); err != nil && err != io.EOF {
		return "", err
	}
	// 使用 http.DetectContentType 根据 Magic Bytes 检测真实 MIME 类型
	contentType := http.DetectContentType(buffer)

	// 重置文件指针到开头，以便后续完整读取文件内容
	if _, err := src.Seek(0, 0); err != nil {
		return "", err
	}

	// 2. 校验 MIME 类型是否在白名单中
	if len(allowedMimes) > 0 {
		isAllowed := false
		for _, mime := range allowedMimes {
			// 使用 HasPrefix 匹配，如 "image/jpeg" 匹配 "image/"
			if strings.HasPrefix(contentType, mime) {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			return "", errorx.Newf(errorx.CodeInvalidParam, "invalid file type: %s", contentType)
		}
	}

	// 3. 生成唯一文件名（雪花ID + 原始扩展名）
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename)) // 获取并转小写的扩展名
	newFileName := snowflake.GenerateIDString() + ext         // 雪花ID保证文件名唯一
	dst := filepath.Join(dstDir, newFileName)                 // 拼接完整目标路径

	// 4. 创建目标文件并写入内容
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close() // 确保函数结束时关闭文件

	// 将源文件内容拷贝到目标文件
	if _, err := io.Copy(out, src); err != nil {
		return "", err
	}

	return newFileName, nil
}

// RecallMessage 撤回消息
// 流程：查消息 → 校验身份 → 校验时限 → 更新数据库 → WebSocket 通知对方
func (m *MessageService) RecallMessage(ctx context.Context, userId string, req messagereq.RecallMessageRequest) error {
	// 1. 查询消息是否存在
	msg, err := m.messageStore.FindByUuid(ctx, req.MessageUuid)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeInvalidParam, "消息不存在")
		}
		zap.L().Error("查询消息失败", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 只有发送者可以撤回
	if msg.SendId != userId {
		return errorx.New(errorx.CodeForbidden, "只能撤回自己发送的消息")
	}

	// 3. 已撤回的消息不能重复撤回
	if msg.Type == int8(msgtype.Recall) {
		return errorx.New(errorx.CodeInvalidParam, "该消息已被撤回")
	}

	// 4. 2分钟时限
	if time.Since(msg.CreatedAt) > 2*time.Minute {
		return errorx.New(errorx.CodeForbidden, "消息发送超过2分钟，无法撤回")
	}

	// 5. 更新消息类型为撤回，清空内容
	if err := m.messageStore.UpdateContent(ctx, req.MessageUuid, "", int8(msgtype.Recall)); err != nil {
		zap.L().Error("撤回消息失败", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 6. 通过 WebSocket 通知（群聊广播所有群成员，私聊通知双方）
	if m.pushRecallNotify != nil {
		m.pushRecallNotify(req.MessageUuid, msg.ReceiveId)
		if len(msg.ReceiveId) > 0 && msg.ReceiveId[0] == 'U' && msg.SendId != "" {
			recallMsg := map[string]interface{}{
				"type":         msgtype.Recall,
				"message_uuid": req.MessageUuid,
			}
			if jsonMsg, err := json.Marshal(recallMsg); err == nil {
				m.trySendBack(msg.SendId, "", jsonMsg)
			}
		}
	}

	// 7. 更新会话最后消息摘要
	m.updateSessionLastMessage(msg, "[一条消息已被撤回]")

	// 8. 同步更新/清除 ZSET 缓存中的撤回消息
	sessionId := msg.SessionId
	if sessionId == "" {
		sessionId = ResolveSessionId(msg.SendId, msg.ReceiveId)
	}
	m.updateRecalledMessageInZSet(sessionId, req.MessageUuid)

	return nil
}

// GetMessageByUuid 根据 UUID 获取消息
func (m *MessageService) GetMessageByUuid(ctx context.Context, messageId string) (*model.Message, error) {
	return m.messageStore.FindByUuid(ctx, messageId)
}

func normalizePath(path string) string {
	if strings.HasPrefix(path, "https://cube.elemecdn.com") {
		return path
	}
	idx := strings.Index(path, "/static/")
	if idx == -1 {
		return path
	}
	return path[idx:]
}

func (m *MessageService) isDuplicateMessage(ctx context.Context, clientMsgId string) (isDuplicate bool, isDegraded bool) {
	if clientMsgId == "" || m.cache == nil {
		return false, false
	}
	dedupKey := "msg:dedup:" + clientMsgId
	ok, err := m.cache.SetNX(ctx, dedupKey, "1", 24*time.Hour)
	if err != nil {
		zap.L().Error("幂等检查失败，降级放行", zap.String("client_msg_id", clientMsgId), zap.Error(err))
		metrics.MessagesDegrade.Inc()
		return false, true
	}
	if !ok {
		metrics.MessagesDuplicated.Inc()
	}
	return !ok, false
}

func (m *MessageService) checkSendPermission(ctx context.Context, sendId, receiveId string) error {
	if len(receiveId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "接收者ID不能为空")
	}

	senderStatus, err := grpc_client.GetUserStatus(ctx, sendId)
	if err == nil && senderStatus == user_status.DISABLE {
		zap.L().Warn("被禁用的用户尝试发送消息", zap.String("sendId", sendId))
		return errorx.New(errorx.CodeForbidden, "您的账号已被禁用，无法发送消息")
	}

	if receiveId[0] == 'U' {
		fsStatus, err := grpc_client.CheckFriendshipStatus(ctx, sendId, receiveId)
		if err == nil && fsStatus != 1 {
			return errorx.New(errorx.CodeForbidden, "你们还不是好友，无法发送消息")
		}
	} else if receiveId[0] == 'G' {
		isMember, err := grpc_client.CheckGroupMember(ctx, receiveId, sendId)
		if err == nil && !isMember {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员，无法发送消息")
		}
	}

	return nil
}

func (m *MessageService) buildMessageFromRequest(req messagereq.ChatMessageRequest) model.Message {
	sessionId := req.SessionId
	if sessionId == "" || (!strings.HasPrefix(sessionId, "P_") && !strings.HasPrefix(sessionId, "G")) {
		sessionId = ResolveSessionId(req.SendId, req.ReceiveId)
	}
	return model.Message{
		Uuid:        "M" + snowflake.GenerateIDString(),
		ClientMsgId: req.ClientMsgId,
		SessionId:   sessionId,
		Type:        req.Type,
		Content:     req.Content,
		Url:         req.Url,
		SendId:      req.SendId,
		ReceiveId:   req.ReceiveId,
		FileSize:    req.FileSize,
		FileType:    req.FileType,
		FileName:    req.FileName,
		Status:      message_status.Unsent,
		AVdata:      req.AVdata,
	}
}

func (m *MessageService) trySendBack(targetUserId string, messageUuid string, payload []byte) {
	if m.cache == nil {
		return
	}
	// 1. 查询目标用户所在的网关 gRPC 实例地址
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	gatewayAddr, err := m.cache.Get(ctx, constants.CacheKeyUserGateway+targetUserId)
	if err != nil || gatewayAddr == "" {
		// 目标用户当前不在线（或尚未在 Redis 登记路由），直接跳过下行推送
		metrics.MessagesDropped.Inc()
		return
	}

	// 2. 存在在线网关路由，通过 gRPC 客户端连接池直推目标网关
	if err := grpc_client.PushToGateway(ctx, gatewayAddr, targetUserId, messageUuid, payload); err != nil {
		zap.L().Warn("push to gateway failed",
			zap.String("userId", targetUserId),
			zap.String("gatewayAddr", gatewayAddr),
			zap.Error(err),
		)
		metrics.MessagesDropped.Inc()
		// 若直推失败（网关失联或用户在该网关已断开），异步清理失效路由
		m.cache.SubmitTask(func() {
			_ = m.cache.Delete(context.Background(), constants.CacheKeyUserGateway+targetUserId)
		})
		return
	}

	metrics.MessagesDispatched.Inc()
}

func (m *MessageService) PushRecallNotify(messageUuid, receiveId string) {
	recallMsg := map[string]interface{}{
		"type":         msgtype.Recall,
		"message_uuid": messageUuid,
	}
	jsonMsg, err := json.Marshal(recallMsg)
	if err != nil {
		zap.L().Error("序列化撤回通知失败", zap.Error(err))
		return
	}

	// 如果是群聊，广播给全体群成员
	if len(receiveId) > 0 && receiveId[0] == 'G' {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		memberIds, err := grpc_client.ListGroupMemberIds(ctx, receiveId)
		if err != nil {
			zap.L().Error("get group members via grpc for recall error", zap.Error(err))
		}
		for _, uid := range memberIds {
			m.trySendBack(uid, "", jsonMsg)
		}
		return
	}

	// 私聊推给接收方
	m.trySendBack(receiveId, "", jsonMsg)
}

func (m *MessageService) updateSessionLastMessage(message *model.Message, content string) {
	if m.sessionStore == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.sessionStore.UpdateLastMessage(ctx,
			message.SendId,
			message.ReceiveId,
			content,
			message.Type,
			message.CreatedAt,
		); err != nil {
			zap.L().Error("更新发送者会话最后消息失败", zap.Error(err))
		}

		if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
			if err := m.sessionStore.UpdateLastMessage(ctx,
				message.ReceiveId,
				message.SendId,
				content,
				message.Type,
				message.CreatedAt,
			); err != nil {
				zap.L().Warn("更新接收者会话最后消息失败", zap.Error(err))
			}
		}
	}()
}

// hydrateMessages 动态通过 RPC 批量聚合发送者公开信息，消除 message 表冗余字段
func (m *MessageService) hydrateMessages(ctx context.Context, messages []model.Message) []messagersp.GetMessageListRespond {
	sendIds := make([]string, 0, len(messages))
	for _, msg := range messages {
		sendIds = append(sendIds, msg.SendId)
	}

	userMap := make(map[string]*userpb.PublicUserInfo)
	if len(sendIds) > 0 {
		if users, err := grpc_client.BatchGetPublicUserInfo(ctx, sendIds); err == nil {
			for _, u := range users {
				userMap[u.Uuid] = u
			}
		} else {
			zap.L().Warn("hydrateMessages: BatchGetPublicUserInfo failed", zap.Error(err))
		}
	}

	rspList := make([]messagersp.GetMessageListRespond, 0, len(messages))
	for _, msg := range messages {
		var sendName, sendAvatar string
		if u, ok := userMap[msg.SendId]; ok {
			sendName = u.Nickname
			sendAvatar = u.Avatar
		}

		rspList = append(rspList, messagersp.GetMessageListRespond{
			SendId:     msg.SendId,
			SendName:   sendName,
			SendAvatar: sendAvatar,
			ReceiveId:  msg.ReceiveId,
			Content:    msg.Content,
			Url:        msg.Url,
			Type:       msg.Type,
			FileType:   msg.FileType,
			FileName:   msg.FileName,
			FileSize:   msg.FileSize,
			CreatedAt:  msg.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return rspList
}

func (m *MessageService) dispatchToUser(message model.Message, sendName, originalAvatar string) {
	messageRsp := messagersp.GetMessageListRespond{
		SendId:     message.SendId,
		SendName:   sendName,
		SendAvatar: originalAvatar,
		ReceiveId:  message.ReceiveId,
		Type:       message.Type,
		Content:    message.Content,
		Url:        message.Url,
		FileSize:   message.FileSize,
		FileName:   message.FileName,
		FileType:   message.FileType,
		CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	jsonMessage, _ := json.Marshal(messageRsp)

	m.trySendBack(message.ReceiveId, message.Uuid, jsonMessage)
	m.trySendBack(message.SendId, message.Uuid, jsonMessage)
}

func (m *MessageService) dispatchToGroup(ctx context.Context, message model.Message, sendName, originalAvatar string) {
	messageRsp := messagersp.GetMessageListRespond{
		SendId:     message.SendId,
		SendName:   sendName,
		SendAvatar: originalAvatar,
		ReceiveId:  message.ReceiveId,
		Type:       message.Type,
		Content:    message.Content,
		Url:        message.Url,
		FileSize:   message.FileSize,
		FileName:   message.FileName,
		FileType:   message.FileType,
		CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	jsonMessage, _ := json.Marshal(messageRsp)

	memberIds, err := grpc_client.ListGroupMemberIds(ctx, message.ReceiveId)
	if err != nil {
		zap.L().Error("get group members via grpc error", zap.Error(err))
	}
	if len(memberIds) == 0 {
		return
	}

	// 异步并发扇出推送，防止大群串行遍历导致发送者请求超时
	go func(members []string, payload []byte, msgUuid string) {
		const maxWorkers = 10
		workers := maxWorkers
		if len(members) < workers {
			workers = len(members)
		}
		ch := make(chan string, len(members))
		for _, uid := range members {
			ch <- uid
		}
		close(ch)

		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for uid := range ch {
					m.trySendBack(uid, msgUuid, payload)
				}
			}()
		}
		wg.Wait()
	}(memberIds, jsonMessage, message.Uuid)
}

func (m *MessageService) dispatchAVToUser(message model.Message, sendName, sendAvatar string) {
	if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
		messageRsp := messagersp.AVMessageRespond{
			SendId:     message.SendId,
			SendName:   sendName,
			SendAvatar: sendAvatar,
			ReceiveId:  message.ReceiveId,
			Type:       message.Type,
			Content:    message.Content,
			Url:        message.Url,
			FileSize:   message.FileSize,
			FileName:   message.FileName,
			FileType:   message.FileType,
			CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
			AVdata:     message.AVdata,
		}
		jsonMessage, _ := json.Marshal(messageRsp)
		m.trySendBack(message.ReceiveId, message.Uuid, jsonMessage)
	}
}

// SendMessage 上行消息发送（同步鉴权、幂等、落库，并向单管道 downstream 投递广播）
func (m *MessageService) SendMessage(ctx context.Context, req messagereq.ChatMessageRequest) (*messagersp.SendMessageRespond, error) {
	// 1. 幂等检查
	isDup, _ := m.isDuplicateMessage(ctx, req.ClientMsgId)
	if isDup {
		return &messagersp.SendMessageRespond{
			MessageUuid: req.ClientMsgId,
			CreatedAt:   time.Now().Format("2006-01-02 15:04:05"),
		}, nil
	}

	// 2. 权限校验（发送者状态、好友关系或群成员关系）
	if err := m.checkSendPermission(ctx, req.SendId, req.ReceiveId); err != nil {
		return nil, err
	}

	message := m.buildMessageFromRequest(req)

	switch req.Type {
	case int8(msgtype.Text):
		metrics.MessagesConsumed.WithLabelValues("text").Inc()
		message.Url = ""
		message.FileSize = "0B"
		message.FileType = ""
		message.FileName = ""
		message.AVdata = ""
		if err := m.messageStore.Create(ctx, &message); err != nil {
			zap.L().Error("创建文本消息失败", zap.Error(err))
			return nil, errorx.ErrServerBusy
		}
		m.cacheMessageToZSet(&message)
		m.updateSessionLastMessage(&message, message.Content)
		if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
			m.dispatchToUser(message, req.SendName, req.SendAvatar)
		} else if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'G' {
			m.dispatchToGroup(ctx, message, req.SendName, req.SendAvatar)
		}

	case int8(msgtype.File):
		metrics.MessagesConsumed.WithLabelValues("file").Inc()
		message.Content = ""
		message.AVdata = ""
		if err := m.messageStore.Create(ctx, &message); err != nil {
			zap.L().Error("创建文件消息失败", zap.Error(err))
			return nil, errorx.ErrServerBusy
		}
		m.cacheMessageToZSet(&message)
		content := "[文件] " + req.FileName
		m.updateSessionLastMessage(&message, content)
		if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'U' {
			m.dispatchToUser(message, req.SendName, req.SendAvatar)
		} else if len(message.ReceiveId) > 0 && message.ReceiveId[0] == 'G' {
			m.dispatchToGroup(ctx, message, req.SendName, req.SendAvatar)
		}

	case int8(msgtype.AudioOrVideo):
		metrics.MessagesConsumed.WithLabelValues("audio_video").Inc()
		var avData messagereq.AVSignalData
		_ = json.Unmarshal([]byte(req.AVdata), &avData)
		message.Content = ""

		if avData.MessageId == "PROXY" && (avData.Type == "start_call" || avData.Type == "receive_call" || avData.Type == "reject_call") {
			if err := m.messageStore.Create(ctx, &message); err == nil {
				m.cacheMessageToZSet(&message)
			}
			var summary string
			switch avData.Type {
			case "start_call":
				summary = "[通话] 发起了通话"
			case "receive_call":
				summary = "[通话] 已接听"
			case "reject_call":
				summary = "[通话] 已拒绝"
			}
			m.updateSessionLastMessage(&message, summary)
		}
		m.dispatchAVToUser(message, req.SendName, req.SendAvatar)
	default:
		return nil, errorx.New(errorx.CodeInvalidParam, "不支持的消息类型")
	}

	return &messagersp.SendMessageRespond{
		MessageUuid: message.Uuid,
		CreatedAt:   message.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

