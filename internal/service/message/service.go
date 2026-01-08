package message

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"kama_chat_server/pkg/util/random"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"kama_chat_server/internal/config"
	"kama_chat_server/internal/dao/mysql/repository"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/errorx"
)

// messageService 消息业务逻辑实现
type messageService struct {
	repos *repository.Repositories
}

// NewMessageService 构造函数
func NewMessageService(repos *repository.Repositories) *messageService {
	return &messageService{repos: repos}
}

// GetMessageList 获取聊天记录
func (m *messageService) GetMessageList(userOneId, userTwoId string) ([]respond.GetMessageListRespond, error) {
	// 确保 ID 顺序一致，保证缓存 Key 唯一
	if userOneId > userTwoId {
		userOneId, userTwoId = userTwoId, userOneId
	}
	cacheKey := "message_list_" + userOneId + "_" + userTwoId

	rspString, err := myredis.GetKeyNilIsErr(context.Background(), cacheKey)
	if err == nil {
		var rsp []respond.GetMessageListRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
			zap.L().Error("json unmarshal cache error", zap.Error(err))
			// 即使缓存解析失败，也尝试查数据库
		} else {
			return rsp, nil
		}
	} else if !errors.Is(err, redis.Nil) {
		// Log error but continue to DB
		zap.L().Error("redis get key error", zap.Error(err))
	}

	// 缓存未命中或出错，查数据库
	messageList, err := m.repos.Message.FindByUserIds(userOneId, userTwoId)
	if err != nil {
		zap.L().Error("find messages by user ids error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	rspList := make([]respond.GetMessageListRespond, 0, len(messageList))
	for _, message := range messageList {
		rspList = append(rspList, respond.GetMessageListRespond{
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

	// 更新缓存
	myredis.SubmitCacheTask(func() {
		jsonBytes, err := json.Marshal(rspList)
		if err != nil {
			zap.L().Error("json marshal error", zap.Error(err))
			return
		}
		if err := myredis.SetKeyEx(context.Background(), cacheKey, string(jsonBytes), time.Duration(constants.REDIS_TIMEOUT)*time.Minute); err != nil {
			zap.L().Error("redis set key error", zap.Error(err))
		}
	})

	return rspList, nil
}

// GetGroupMessageList 获取群聊消息记录
func (m *messageService) GetGroupMessageList(groupId string) ([]respond.GetGroupMessageListRespond, error) {
	cacheKey := "group_messagelist_" + groupId
	rspString, err := myredis.GetKeyNilIsErr(context.Background(), cacheKey)
	if err == nil {
		var rsp []respond.GetGroupMessageListRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err != nil {
			zap.L().Error("json unmarshal cache error", zap.Error(err))
		} else {
			return rsp, nil
		}
	} else if !errors.Is(err, redis.Nil) {
		zap.L().Error("redis get key error", zap.Error(err))
	}

	messageList, err := m.repos.Message.FindByGroupId(groupId)
	if err != nil {
		zap.L().Error("find group messages error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	rspList := make([]respond.GetGroupMessageListRespond, 0, len(messageList))
	for _, message := range messageList {
		rspList = append(rspList, respond.GetGroupMessageListRespond{
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

	// 更新缓存
	myredis.SubmitCacheTask(func() {
		jsonBytes, err := json.Marshal(rspList)
		if err != nil {
			zap.L().Error("json marshal error", zap.Error(err))
			return
		}
		if err := myredis.SetKeyEx(context.Background(), cacheKey, string(jsonBytes), time.Duration(constants.REDIS_TIMEOUT)*time.Minute); err != nil {
			zap.L().Error("redis set key error", zap.Error(err))
		}
	})

	return rspList, nil
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
	newFileName := random.GetNowAndLenRandomString(10) + ext
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
