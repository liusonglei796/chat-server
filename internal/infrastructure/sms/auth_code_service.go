package sms

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dysmsapi20170525 "github.com/alibabacloud-go/dysmsapi-20170525/v4/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"go.uber.org/zap"

	"kama_chat_server/internal/config"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/random"
)

// SmsService 短信服务接口
// 抽象短信发送操作，支持多种实现（阿里云、本地 mock 等）
// Service 层应依赖此接口而非具体实现
type SmsService interface {
	// SendVerificationCode 发送短信验证码
	SendVerificationCode(telephone string) error
}

type localSmsService struct {
	cache myredis.CacheService
}

func (s *localSmsService) SendVerificationCode(telephone string) error {
	key := "auth_code_" + telephone
	code, err := s.cache.Get(context.Background(), key)
	if err != nil {
		zap.L().Error("缓存频率检查异常", zap.Error(err), zap.String("phone", telephone))
		return errorx.ErrServerBusy
	}
	if code != "" {
		return errorx.New(errorx.CodeInvalidParam, "目前还不能发送验证码，请稍后重试或输入已发送的验证码")
	}

	code = strconv.Itoa(random.GetRandomInt(6))
	fmt.Printf("【MockSMS】手机号: %s, 验证码: %s\n", telephone, code)

	if err := s.cache.Set(context.Background(), key, code, time.Minute); err != nil {
		zap.L().Error("缓存写入验证码失败", zap.Error(err))
		return errorx.ErrServerBusy
	}

	return nil
}

func shouldUseMock(auth config.AuthCodeConfig) bool {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("KAMACHAT_SMS_MODE")))
	if mode == "mock" || mode == "local" || mode == "test" {
		return true
	}
	// configs/config.toml 默认是占位字符串；没配真实 AK 时默认走 mock，便于本机跑通注册/短信登录链路
	ak := strings.ToLower(strings.TrimSpace(auth.AccessKeyID))
	ask := strings.ToLower(strings.TrimSpace(auth.AccessKeySecret))
	if ak == "" || ask == "" {
		return true
	}
	if strings.Contains(ak, "your accesskey") || strings.Contains(ask, "your accesskey") {
		return true
	}
	return false
}

// aliyunSmsService 阿里云短信服务实现
// 实现 SmsService 接口，遵循依赖倒置原则
type aliyunSmsService struct {
	client *dysmsapi20170525.Client
	cache  myredis.CacheService // 依赖抽象接口而非具体 Redis 实现
}



// Init 初始化阿里云 SMS Client 并创建服务实例
// cacheService: 缓存服务接口实例（用于频率限制和验证码存储）
func Init(cacheService myredis.CacheService) (SmsService, error) {
	authCfg := config.GetConfig().AuthCodeConfig
	if shouldUseMock(authCfg) {
		zap.L().Warn("SMS Service 使用本地 Mock 模式（仅写入 Redis，不调用第三方短信）")
		return &localSmsService{cache: cacheService}, nil
	}

	conf := &openapi.Config{
		AccessKeyId:     tea.String(authCfg.AccessKeyID),
		AccessKeySecret: tea.String(authCfg.AccessKeySecret),
	}
	conf.Endpoint = tea.String("dysmsapi.aliyuncs.com")
	client, err := dysmsapi20170525.NewClient(conf)
	if err != nil {
		zap.L().Error("Aliyun SMS Client Init Failed", zap.Error(err))
		return nil, err
	}

	return &aliyunSmsService{client: client, cache: cacheService}, nil
}

// NewAliyunSmsService 创建阿里云短信服务实例（用于依赖注入）
func NewAliyunSmsService(client *dysmsapi20170525.Client, cacheService myredis.CacheService) SmsService {
	return &aliyunSmsService{
		client: client,
		cache:  cacheService,
	}
}

// SendVerificationCode 发送验证码核心逻辑（实现 SmsService 接口）
// 包含：频率限制检查、验证码生成、缓存预存、阿里云 API 调用以及失败回滚机制
func (s *aliyunSmsService) SendVerificationCode(telephone string) error {
	// 1. 安全检查：确保短信客户端已初始化
	if s.client == nil {
		zap.L().Error("短信服务调用失败：smsClient 未初始化")
		return errorx.New(errorx.CodeServerBusy, "短信服务未初始化")
	}

	// 2. 频率限制检查 (Throttling)
	// 通过缓存接口查询该手机号是否已有未过期的验证码
	key := "auth_code_" + telephone
	code, err := s.cache.Get(context.Background(), key)
	if err != nil {
		zap.L().Error("缓存频率检查异常", zap.Error(err), zap.String("phone", telephone))
		return errorx.ErrServerBusy
	}

	// 如果 code 不为空，说明 1 分钟内已发送过，需拦截请求，防止短信资源被恶意浪费
	if code != "" {
		return errorx.New(errorx.CodeInvalidParam, "目前还不能发送验证码，请稍后重试或输入已发送的验证码")
	}

	// 3. 生成验证码：生成 6 位纯数字随机字符串
	code = strconv.Itoa(random.GetRandomInt(6))
	// 开发环境调试使用，生产环境应通过日志等级控制或移除
	fmt.Printf("【Debug】手机号: %s, 生成验证码: %s\n", telephone, code)

	// 4. 预存缓存：设置 1 分钟有效期
	// 先占位，后发送。如果先发送后占位，在极高并发下可能被绕过频率限制
	if err := s.cache.Set(context.Background(), key, code, time.Minute); err != nil {
		zap.L().Error("缓存写入验证码失败", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 5. 配置加载与兜底 (Fallback)
	// 优先使用配置文件中的签名和模板 ID，若未配置则使用阿里云提供的测试模板
	authConfig := config.GetConfig().AuthCodeConfig
	signName := authConfig.SignName
	if signName == "" {
		signName = "阿里云短信测试"
	}
	templateCode := authConfig.TemplateCode
	if templateCode == "" {
		templateCode = "SMS_154950909"
	}

	// 6. 构造阿里云发送请求
	sendSmsRequest := &dysmsapi20170525.SendSmsRequest{
		SignName:     tea.String(signName),
		TemplateCode: tea.String(templateCode),
		PhoneNumbers: tea.String(telephone),
		// 对应模板中的变量 ${code}
		TemplateParam: tea.String("{\"code\":\"" + code + "\"}"),
	}

	// 7. 执行发送操作
	runtime := &util.RuntimeOptions{}
	rsp, err := s.client.SendSmsWithOptions(sendSmsRequest, runtime)

	// 8. 异常处理与事务回滚
	if err != nil {
		zap.L().Error("调用阿里云短信接口发生系统级错误", zap.Error(err))

		// 【关键逻辑】回滚：如果发送失败，必须删除缓存中的占位 Key
		// 否则用户在接下来的 1 分钟内无法再次触发发送请求，体验极差
		_ = s.cache.Delete(context.Background(), key)

		return errorx.ErrServerBusy
	}

	// 9. 记录发送详情（包含阿里云返回的业务错误码）
	// 注意：即使 err 为 nil，也需要看 rsp.Body.Code 是否为 "OK"
	zap.L().Info("短信发送接口响应", zap.String("response", *util.ToJSONString(rsp)))

	return nil
}


