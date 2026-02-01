package message

import (
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kama_chat_server/internal/config"
	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	messagersp "kama_chat_server/internal/dto/respond/message"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/errorx"
)

// messageService 消息业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖，遵循依赖倒置原则
type messageService struct {
	repos *mysql.Repositories
	cache myredis.AsyncCacheService
}

// NewMessageService 构造函数，注入所有依赖
func NewMessageService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *messageService {
	return &messageService{
		repos: repos,
		cache: cacheService,
	}
}

// GetMessageList 获取聊天记录（分页）
func (m *messageService) GetMessageList(requesterId, partnerId string, page, pageSize int) ([]messagersp.GetMessageListRespond, error) {
	userOneId := requesterId

	// 确保 ID 顺序一致，保证缓存 Key 唯一
	if userOneId > partnerId {
		userOneId, partnerId = partnerId, userOneId
	}

	// 计算分页偏移量
	offset := (page - 1) * pageSize

	// 查数据库（带分页）
	messageList, err := m.repos.Message.FindByUserIdsPaged(userOneId, partnerId, offset, pageSize)
	if err != nil {
		zap.L().Error("find messages by user ids error", zap.Error(err))
		return nil, errorx.ErrServerBusy
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

	return rspList, nil
}

// GetGroupMessageList 获取群聊消息记录（分页）(userId 必须是群成员)
func (m *messageService) GetGroupMessageList(userId, groupId string, page, pageSize int) ([]messagersp.GetMessageListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// 权限校验: 只要有 Session 记录(未删除)即可查看历史消息，不仅仅是当前成员
	// 这样可以支持"退群后查看历史消息"的需求
	_, err := m.repos.Session.FindBySendIdAndReceiveId(userId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, 0, errorx.New(errorx.CodeForbidden, "您没有该群的会话记录")
		}
		zap.L().Error("Find session error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	// 分页查询数据库
	messageList, total, err := m.repos.Message.FindByGroupIdPaged(groupId, offset, pageSize)
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

// UploadAvatar 上传头像
func (m *messageService) UploadAvatar(c *gin.Context) (string, error) {
	if err := c.Request.ParseMultipartForm(constants.FILE_MAX_SIZE); err != nil {
		zap.L().Error("parse multipart form error", zap.Error(err))
		return "", errorx.ErrServerBusy
	}
	mForm := c.Request.MultipartForm
	if len(mForm.File) == 0 {
		return "", errorx.New(errorx.CodeInvalidParam, "no file uploaded")
	}

	// 遍历所有文件，但既然是上传头像，通常只取第一个
	for _, headers := range mForm.File {
		for _, fileHeader := range headers {
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
func (m *messageService) UploadFile(c *gin.Context) ([]string, error) {
	if err := c.Request.ParseMultipartForm(constants.FILE_MAX_SIZE); err != nil {
		zap.L().Error("parse multipart form error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	var uploadedFiles []string
	dstDir := config.GetConfig().StaticFilePath
	mForm := c.Request.MultipartForm

	for _, headers := range mForm.File {
		for _, fileHeader := range headers {
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
func (m *messageService) saveFile(fileHeader *multipart.FileHeader, dstDir string, allowedMimes ...string) (string, error) {
	src, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	// 1. 读取前 512 字节进行 MIME 类型的 Magic Bytes 校验
	buffer := make([]byte, 512)
	if _, err := src.Read(buffer); err != nil && err != io.EOF {
		return "", err
	}
	contentType := http.DetectContentType(buffer)

	// 重置文件指针
	if _, err := src.Seek(0, 0); err != nil {
		return "", err
	}

	// 2. 校验 MIME 类型
	if len(allowedMimes) > 0 {
		isAllowed := false
		for _, mime := range allowedMimes {
			if strings.HasPrefix(contentType, mime) {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			return "", errorx.Newf(errorx.CodeInvalidParam, "invalid file type: %s", contentType)
		}
	}

	// 3. 生成唯一文件名
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	newFileName := snowflake.GenerateIDString() + ext
	dst := filepath.Join(dstDir, newFileName)

	// 4. 保存文件
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		return "", err
	}

	return newFileName, nil
}
