package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/config"
	"kama_chat_server/internal/dao/mysql"
	aiclient "kama_chat_server/internal/infrastructure/ai"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/errorx"

	aireq "kama_chat_server/internal/dto/request/ai"
	airsp "kama_chat_server/internal/dto/respond/ai"
)

// AiService AI 业务服务
type AiService struct {
	repos  *mysql.Repositories
	client aiclient.ChatClient
	model  string
}

// NewAIService 创建 AI 业务服务
func NewAIService(repos *mysql.Repositories, cfg *config.Config) *AiService {
	client, err := aiclient.NewEinoClient(cfg.ModelScopeConfig)
	if err != nil {
		zap.L().Warn("init eino client failed, ai service disabled", zap.Error(err))
	}

	return &AiService{
		repos:  repos,
		client: client,
		model:  cfg.ModelScopeConfig.Model,
	}
}

// ReplySuggestions 智能回复建议
func (s *AiService) ReplySuggestions(ctx context.Context, userId string, req aireq.ReplySuggestionsRequest) (*airsp.ReplySuggestionsRespond, error) {
	if err := s.ensureClientReady(); err != nil {
		return nil, err
	}

	targetId := strings.TrimSpace(req.TargetId)
	if targetId == "" {
		return nil, errorx.New(errorx.CodeInvalidParam, "target_id 不能为空")
	}

	if req.ContextLimit <= 0 {
		req.ContextLimit = constants.DefaultReplyContextLimit
	}
	if req.ContextLimit > constants.MaxReplyContextLimit {
		req.ContextLimit = constants.MaxReplyContextLimit
	}

	conversation, err := s.buildConversationContext(ctx, userId, targetId, req.ContextLimit)
	if err != nil {
		return nil, err
	}

	style := strings.TrimSpace(req.Style)
	if style == "" {
		style = "brief"
	}

	systemPrompt := "你是聊天助手，任务是给出3条可直接发送的中文回复建议。只输出 JSON，格式：{\"suggestions\":[\"...\",\"...\",\"...\"]}。不要输出额外文字。"
	userPrompt := fmt.Sprintf("聊天上下文：\n%s\n\n用户草稿：%s\n风格：%s\n要求：每条回复不超过40字，语气自然，避免重复。", conversation, strings.TrimSpace(req.Draft), style)

	aiCtx, cancel := context.WithTimeout(ctx, constants.AIRequestTimeout)
	defer cancel()

	// 调用 Eino 客户端生成回复建议：
	// systemPrompt 约束输出必须是 JSON；userPrompt 提供聊天上下文、草稿和风格。
	out, err := s.client.Generate(aiCtx, systemPrompt, userPrompt)
	if err != nil {
		zap.L().Error("ai reply suggestions failed", zap.Error(err), zap.String("user_id", userId), zap.String("target_id", targetId))
		return nil, errorx.ErrServerBusy
	}

	suggestions := parseSuggestions(out)
	if len(suggestions) == 0 {
		return nil, errorx.New(errorx.CodeServerBusy, "AI 返回内容不可用，请稍后重试")
	}
	if len(suggestions) > 3 {
		suggestions = suggestions[:3]
	}

	return &airsp.ReplySuggestionsRespond{Suggestions: suggestions}, nil
}

// GroupSummary 群聊总结
func (s *AiService) GroupSummary(ctx context.Context, userId string, req aireq.GroupSummaryRequest) (*airsp.GroupSummaryRespond, error) {
	if err := s.ensureClientReady(); err != nil {
		return nil, err
	}

	groupId := strings.TrimSpace(req.GroupId)
	if groupId == "" {
		return nil, errorx.New(errorx.CodeInvalidParam, "group_id 不能为空")
	}

	if !strings.HasPrefix(groupId, "G") {
		return nil, errorx.New(errorx.CodeInvalidParam, "group_id 格式错误")
	}

	if req.Hours <= 0 {
		req.Hours = constants.DefaultSummaryHours
	}
	if req.Limit <= 0 {
		req.Limit = constants.DefaultSummaryLimit
	}
	if req.Limit > constants.MaxSummaryLimit {
		req.Limit = constants.MaxSummaryLimit
	}

	if _, err := s.repos.GroupMember.FindByGroupAndUser(ctx, groupId, userId); err != nil {
		if errorx.IsNotFound(err) {
			return nil, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("check group member failed", zap.Error(err), zap.String("group_id", groupId), zap.String("user_id", userId))
		return nil, errorx.ErrServerBusy
	}

	result, err := s.repos.Message.FindByGroupIdCursor(ctx, groupId, "", req.Limit)
	if err != nil {
		zap.L().Error("load group messages failed", zap.Error(err), zap.String("group_id", groupId))
		return nil, errorx.ErrServerBusy
	}

	since := time.Now().Add(-time.Duration(req.Hours) * time.Hour)
	conversation := s.messagesToContext(filterMessagesBySince(result.Messages, since))
	if strings.TrimSpace(conversation) == "" {
		return &airsp.GroupSummaryRespond{
			Summary:   "该时间段内暂无可总结消息。",
			Todos:     []string{},
			Decisions: []string{},
		}, nil
	}

	style := strings.TrimSpace(req.Style)
	if style == "" {
		style = "brief"
	}

	systemPrompt := "你是群聊总结助手。只输出 JSON，格式：{\"summary\":\"...\",\"todos\":[\"...\"],\"decisions\":[\"...\"]}。不要输出额外文字。"
	userPrompt := fmt.Sprintf("请对以下群聊内容做%s总结，提炼重点、待办和决策：\n%s", style, conversation)

	aiCtx, cancel := context.WithTimeout(ctx, constants.AIRequestTimeout)
	defer cancel()

	// 调用 Eino 客户端生成群聊总结：
	// 模型返回 JSON（summary/todos/decisions），再由 parseGroupSummary 做结构化解析。
	out, err := s.client.Generate(aiCtx, systemPrompt, userPrompt)
	if err != nil {
		zap.L().Error("ai group summary failed", zap.Error(err), zap.String("group_id", groupId), zap.String("user_id", userId))
		return nil, errorx.ErrServerBusy
	}

	summary := parseGroupSummary(out)
	if strings.TrimSpace(summary.Summary) == "" {
		summary.Summary = "总结结果为空，请稍后重试。"
	}

	return &summary, nil
}

// Translate 多语言翻译
func (s *AiService) Translate(ctx context.Context, userId string, req aireq.TranslateRequest) (*airsp.TranslateRespond, error) {
	if err := s.ensureClientReady(); err != nil {
		return nil, err
	}

	text := strings.TrimSpace(req.Text)
	targetLang := strings.TrimSpace(req.TargetLang)
	sourceLang := strings.TrimSpace(req.SourceLang)

	if text == "" {
		return nil, errorx.New(errorx.CodeInvalidParam, "text 不能为空")
	}
	if targetLang == "" {
		return nil, errorx.New(errorx.CodeInvalidParam, "target_lang 不能为空")
	}

	systemPrompt := "你是翻译助手。只输出 JSON，格式：{\"detected_lang\":\"xx\",\"translated_text\":\"...\"}。不要输出额外文字。"
	userPrompt := fmt.Sprintf("原文：%s\n源语言：%s（若为空请自动识别）\n目标语言：%s\n要求：保持人名、群名和术语不被误译。", text, sourceLang, targetLang)

	aiCtx, cancel := context.WithTimeout(ctx, constants.AIRequestTimeout)
	defer cancel()

	// 调用 Eino 客户端执行翻译：
	// 模型按约定返回 detected_lang 和 translated_text 两个字段。
	out, err := s.client.Generate(aiCtx, systemPrompt, userPrompt)
	if err != nil {
		zap.L().Error("ai translate failed", zap.Error(err), zap.String("user_id", userId))
		return nil, errorx.ErrServerBusy
	}

	rsp := parseTranslate(out, sourceLang)
	if strings.TrimSpace(rsp.TranslatedText) == "" {
		return nil, errorx.New(errorx.CodeServerBusy, "翻译结果为空，请稍后重试")
	}

	return &rsp, nil
}

func (s *AiService) ensureClientReady() error {
	if s.client == nil {
		return errorx.New(errorx.CodeServerBusy, "AI 服务未配置，请先设置 MODELSCOPE_API_KEY")
	}
	return nil
}

func (s *AiService) buildConversationContext(ctx context.Context, userId, targetId string, limit int) (string, error) {
	if strings.HasPrefix(targetId, "G") {
		if _, err := s.repos.Session.FindBySendIdAndReceiveId(ctx, userId, targetId); err != nil {
			if errorx.IsNotFound(err) {
				return "", errorx.New(errorx.CodeForbidden, "你没有该群会话记录")
			}
			zap.L().Error("find group session failed", zap.Error(err), zap.String("user_id", userId), zap.String("group_id", targetId))
			return "", errorx.ErrServerBusy
		}

		result, err := s.repos.Message.FindByGroupIdCursor(ctx, targetId, "", limit)
		if err != nil {
			zap.L().Error("load group context failed", zap.Error(err), zap.String("group_id", targetId))
			return "", errorx.ErrServerBusy
		}
		return s.messagesToContext(result.Messages), nil
	}

	if targetId != userId {
		isFriend, err := s.repos.Friendship.IsFriend(ctx, userId, targetId)
		if err != nil {
			zap.L().Error("check friend relationship failed", zap.Error(err), zap.String("user_id", userId), zap.String("target_id", targetId))
			return "", errorx.ErrServerBusy
		}
		if !isFriend {
			return "", errorx.New(errorx.CodeForbidden, "你们不是好友，无法获取对话上下文")
		}
	}

	userOneId := userId
	userTwoId := targetId
	if userOneId > userTwoId {
		userOneId, userTwoId = userTwoId, userOneId
	}

	result, err := s.repos.Message.FindByUserIdsCursor(ctx, userOneId, userTwoId, "", limit)
	if err != nil {
		zap.L().Error("load private context failed", zap.Error(err), zap.String("user_one", userOneId), zap.String("user_two", userTwoId))
		return "", errorx.ErrServerBusy
	}

	return s.messagesToContext(result.Messages), nil
}

func (s *AiService) messagesToContext(messages []model.Message) string {
	if len(messages) == 0 {
		return ""
	}

	ordered := make([]model.Message, len(messages))
	copy(ordered, messages)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})

	lines := make([]string, 0, len(ordered))
	for _, msg := range ordered {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			if strings.TrimSpace(msg.FileName) != "" {
				content = "[文件] " + msg.FileName
			} else if strings.TrimSpace(msg.Url) != "" {
				content = "[链接] " + msg.Url
			} else {
				content = "[空内容]"
			}
		}
		if len(content) > 180 {
			content = content[:180]
		}
		name := strings.TrimSpace(msg.SendName)
		if name == "" {
			name = msg.SendId
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", msg.CreatedAt.Format("01-02 15:04"), name, content))
	}

	return strings.Join(lines, "\n")
}

func filterMessagesBySince(messages []model.Message, since time.Time) []model.Message {
	if len(messages) == 0 {
		return messages
	}
	filtered := make([]model.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.CreatedAt.After(since) {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}

func parseSuggestions(content string) []string {
	type suggestionsJSON struct {
		Suggestions []string `json:"suggestions"`
	}

	var parsed suggestionsJSON
	if tryParseJSON(content, &parsed) && len(parsed.Suggestions) > 0 {
		return normalizeList(parsed.Suggestions)
	}

	lines := strings.Split(content, "\n")
	out := make([]string, 0, 3)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "-•")
		line = strings.TrimSpace(strings.TrimLeft(line, "0123456789.、)"))
		if line != "" {
			out = append(out, line)
		}
		if len(out) == 3 {
			break
		}
	}
	return normalizeList(out)
}

func parseGroupSummary(content string) airsp.GroupSummaryRespond {
	type summaryJSON struct {
		Summary   string   `json:"summary"`
		Todos     []string `json:"todos"`
		Decisions []string `json:"decisions"`
	}

	var parsed summaryJSON
	if tryParseJSON(content, &parsed) {
		return airsp.GroupSummaryRespond{
			Summary:   strings.TrimSpace(parsed.Summary),
			Todos:     normalizeList(parsed.Todos),
			Decisions: normalizeList(parsed.Decisions),
		}
	}

	return airsp.GroupSummaryRespond{
		Summary:   strings.TrimSpace(content),
		Todos:     []string{},
		Decisions: []string{},
	}
}

func parseTranslate(content, fallbackSourceLang string) airsp.TranslateRespond {
	type translateJSON struct {
		DetectedLang   string `json:"detected_lang"`
		TranslatedText string `json:"translated_text"`
	}

	var parsed translateJSON
	if tryParseJSON(content, &parsed) {
		detected := strings.TrimSpace(parsed.DetectedLang)
		if detected == "" {
			detected = strings.TrimSpace(fallbackSourceLang)
		}
		if detected == "" {
			detected = "unknown"
		}
		return airsp.TranslateRespond{
			DetectedLang:   detected,
			TranslatedText: strings.TrimSpace(parsed.TranslatedText),
		}
	}

	detected := strings.TrimSpace(fallbackSourceLang)
	if detected == "" {
		detected = "unknown"
	}

	return airsp.TranslateRespond{
		DetectedLang:   detected,
		TranslatedText: strings.TrimSpace(content),
	}
}

func tryParseJSON(content string, out any) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}

	if err := json.Unmarshal([]byte(content), out); err == nil {
		return true
	}

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(content[start:end+1]), out); err == nil {
			return true
		}
	}

	return false
}

func normalizeList(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}
