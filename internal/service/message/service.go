package message

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kama_chat_server/internal/config"

	"kama_chat_server/internal/domain/repository"
	messagereq "kama_chat_server/internal/dto/request/message"
	messagersp "kama_chat_server/internal/dto/respond/message"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	msgtype "kama_chat_server/pkg/enum/message/message_type"
	"kama_chat_server/pkg/errorx"
)

// MessageService 消息业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖，遵循依赖倒置原则
type MessageService struct {
	messageRepo    repository.MessageRepository
	friendshipRepo repository.FriendshipRepository
	sessionRepo    repository.SessionRepository
	cache          repository.AsyncCacheService
	// pushRecallNotify 撤回通知回调（由 ChatServer.Broker.PushRecallNotify 提供）
	// 用于在撤回成功后通过 WebSocket 实时通知对方
	pushRecallNotify func(messageUuid, receiveId string)
}

// NewMessageService 构造函数，注入所有依赖
// pushRecallNotify: 可选回调，撤回消息后通过 WebSocket 推送通知
func NewMessageService(
	messageRepo repository.MessageRepository,
	friendshipRepo repository.FriendshipRepository,
	sessionRepo repository.SessionRepository,
	cacheService repository.AsyncCacheService,
	pushRecallNotify func(messageUuid, receiveId string),
) *MessageService {
	return &MessageService{
		messageRepo:      messageRepo,
		friendshipRepo:   friendshipRepo,
		sessionRepo:      sessionRepo,
		cache:            cacheService,
		pushRecallNotify: pushRecallNotify,
	}
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
	isFriend, err := m.friendshipRepo.IsFriend(ctx, requesterId, partnerId)
	if err != nil {
		zap.L().Error("check friend relationship error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}
	if !isFriend {
		return nil, 0, errorx.New(errorx.CodeForbidden, "你们不是好友，无法查看聊天记录")
	}

	userOneId := requesterId

	// 确保 ID 顺序一致，保证查询结果稳定
	if userOneId > partnerId {
		userOneId, partnerId = partnerId, userOneId
	}

	// 查数据库（带分页）
	messageList, total, err := m.messageRepo.FindByUserIdsPaged(ctx, userOneId, partnerId, page, pageSize)
	if err != nil {
		zap.L().Error("find messages by user ids error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	rspList := make([]messagersp.GetMessageListRespond, 0, len(messageList))
	for _, message := range messageList {
		rspList = append(rspList, messagersp.GetMessageListRespond{
			SendId:     message.SendId,
			SendName:   message.SendName,
			SendAvatar: message.SendAvatar,
			ReceiveId:  message.ReceiveId,
			Content:    message.Content,
			Url:        message.Url,
			Type:       message.Type,
			FileType:   message.FileType,
			FileName:   message.FileName,
			FileSize:   message.FileSize,
			CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return rspList, total, nil
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
	_, err := m.sessionRepo.FindBySendIdAndReceiveId(ctx, userId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, 0, errorx.New(errorx.CodeForbidden, "您没有该群的会话记录")
		}
		zap.L().Error("Find session error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	// 分页查询数据库
	messageList, total, err := m.messageRepo.FindByGroupIdPaged(ctx, groupId, page, pageSize)
	if err != nil {
		zap.L().Error("find group messages error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	rspList := make([]messagersp.GetMessageListRespond, 0, len(messageList))
	for _, message := range messageList {
		rspList = append(rspList, messagersp.GetMessageListRespond{
			SendId:     message.SendId,
			SendName:   message.SendName,
			SendAvatar: message.SendAvatar,
			ReceiveId:  message.ReceiveId,
			Content:    message.Content,
			Url:        message.Url,
			Type:       message.Type,
			FileType:   message.FileType,
			FileName:   message.FileName,
			FileSize:   message.FileSize,
			CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return rspList, total, nil
}

// GetMessageListCursor 获取两个用户之间的聊天记录（游标分页）
// cursor: 上一页最后一条消息的时间戳（Unix时间戳字符串），为空则从头开始
func (m *MessageService) GetMessageListCursor(ctx context.Context, requesterId, partnerId, cursor string, pageSize int) ([]messagersp.GetMessageListRespond, string, bool, error) {
	// 参数校验
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 权限校验: 必须是好友关系才能查看聊天记录
	isFriend, err := m.friendshipRepo.IsFriend(ctx, requesterId, partnerId)
	if err != nil {
		zap.L().Error("check friend relationship error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}
	if !isFriend {
		return nil, "", false, errorx.New(errorx.CodeForbidden, "你们不是好友，无法查看聊天记录")
	}

	userOneId := requesterId

	// 确保 ID 顺序一致，保证查询结果稳定
	if userOneId > partnerId {
		userOneId, partnerId = partnerId, userOneId
	}

	// 游标分页查询
	result, err := m.messageRepo.FindByUserIdsCursor(ctx, userOneId, partnerId, cursor, pageSize)
	if err != nil {
		zap.L().Error("find messages by user ids cursor error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	rspList := make([]messagersp.GetMessageListRespond, 0, len(result.Messages))
	for _, message := range result.Messages {
		rspList = append(rspList, messagersp.GetMessageListRespond{
			SendId:     message.SendId,
			SendName:   message.SendName,
			SendAvatar: message.SendAvatar,
			ReceiveId:  message.ReceiveId,
			Content:    message.Content,
			Url:        message.Url,
			Type:       message.Type,
			FileType:   message.FileType,
			FileName:   message.FileName,
			FileSize:   message.FileSize,
			CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return rspList, result.NextCursor, result.HasMore, nil
}

// GetGroupMessageListCursor 获取群聊消息记录（游标分页）
// cursor: 上一页最后一条消息的时间戳（Unix时间戳字符串），为空则从头开始
func (m *MessageService) GetGroupMessageListCursor(ctx context.Context, userId, groupId, cursor string, pageSize int) ([]messagersp.GetMessageListRespond, string, bool, error) {
	// 设置默认分页参数
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 权限校验: 只要有 Session 记录(未删除)即可查看历史消息
	_, err := m.sessionRepo.FindBySendIdAndReceiveId(ctx, userId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, "", false, errorx.New(errorx.CodeForbidden, "您没有该群的会话记录")
		}
		zap.L().Error("Find session error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	// 游标分页查询数据库
	result, err := m.messageRepo.FindByGroupIdCursor(ctx, groupId, cursor, pageSize)
	if err != nil {
		zap.L().Error("find group messages cursor error", zap.Error(err))
		return nil, "", false, errorx.ErrServerBusy
	}

	rspList := make([]messagersp.GetMessageListRespond, 0, len(result.Messages))
	for _, message := range result.Messages {
		rspList = append(rspList, messagersp.GetMessageListRespond{
			SendId:     message.SendId,
			SendName:   message.SendName,
			SendAvatar: message.SendAvatar,
			ReceiveId:  message.ReceiveId,
			Content:    message.Content,
			Url:        message.Url,
			Type:       message.Type,
			FileType:   message.FileType,
			FileName:   message.FileName,
			FileSize:   message.FileSize,
			CreatedAt:  message.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return rspList, result.NextCursor, result.HasMore, nil
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
	msg, err := m.messageRepo.FindByUuid(ctx, req.MessageUuid)
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
	if err := m.messageRepo.UpdateContent(ctx, req.MessageUuid, "", int8(msgtype.Recall)); err != nil {
		zap.L().Error("撤回消息失败", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 6. 通过 WebSocket 通知接收方（对方在线则实时收到，不在线则忽略）
	if m.pushRecallNotify != nil {
		m.pushRecallNotify(req.MessageUuid, msg.ReceiveId)
	}

	return nil
}

// GetMessageByUuid 根据 UUID 获取消息
func (m *MessageService) GetMessageByUuid(ctx context.Context, messageId string) (*model.Message, error) {
	return m.messageRepo.FindByUuid(ctx, messageId)
}
