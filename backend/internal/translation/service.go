package translation

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"bangumipipeline.local/server/internal/system"
)

const TaskKey = "translate-anime-metadata"

type SettingsProvider interface {
	GetLLMSettings(context.Context) (system.LLMSettings, error)
	GetNetworkSettings(context.Context) (system.NetworkSettings, error)
}

type Service struct {
	db       *sql.DB
	settings SettingsProvider
	logger   *slog.Logger
	now      func() time.Time
}

type ConnectionTestResult struct {
	Response string `json:"response"`
}

type targetKind string

const (
	targetAnimeTitle        targetKind = "anime_title"
	targetAnimeSummary      targetKind = "anime_summary"
	targetEpisodeTitle      targetKind = "episode_title"
	targetEpisodeSummary    targetKind = "episode_summary"
	targetCharacterSummary  targetKind = "character_summary"
	targetActorShortSummary targetKind = "actor_short_summary"
)

type translationTarget struct {
	Kind       targetKind
	ID         int64
	BangumiID  int64
	AnimeTitle string
	TextType   string
	SourceText string
}

type chatClient struct {
	settings system.LLMSettings
	client   *http.Client
}

func NewService(db *sql.DB, settings SettingsProvider, logger *slog.Logger) *Service {
	return &Service{db: db, settings: settings, logger: logger, now: time.Now}
}

func (s *Service) Execute(ctx context.Context) error {
	settings, err := s.settings.GetLLMSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Configured() {
		return errors.New("LLM 设置未配置，请先在系统设置中填写 Base URL 和模型名称")
	}
	network, err := s.settings.GetNetworkSettings(ctx)
	if err != nil {
		return err
	}
	client, err := newChatClient(settings, network)
	if err != nil {
		return err
	}

	translated := 0
	copied := 0
	for {
		target, ok, err := s.nextTarget(ctx)
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var text string
		copiedDirectly := false
		if canCopyWithoutLLM(target) {
			text = strings.TrimSpace(target.SourceText)
			copiedDirectly = true
			copied++
		} else {
			text, err = client.translate(ctx, target)
			if err != nil {
				return fmt.Errorf("翻译%s失败: %w", target.TextType, err)
			}
			translated++
		}
		if err := s.saveTranslation(ctx, target, text); err != nil {
			return err
		}
		s.logger.Info("元数据翻译完成",
			"source", "translation",
			"text_type", target.TextType,
			"anime_title", target.AnimeTitle,
			"bangumi_id", target.BangumiID,
			"target_id", target.ID,
			"copied_directly", copiedDirectly,
			"original_text", target.SourceText,
			"translated_text", text,
		)
	}
	s.logger.Info("新番元数据翻译任务完成", "source", "translation", "translated", translated, "copied", copied)
	return nil
}

func (s *Service) TestConnection(ctx context.Context, settings system.LLMSettings) (ConnectionTestResult, error) {
	settings.BaseURL = strings.TrimSpace(settings.BaseURL)
	settings.APIKey = strings.TrimSpace(settings.APIKey)
	settings.Model = strings.TrimSpace(settings.Model)
	if !settings.Configured() {
		return ConnectionTestResult{}, errors.New("LLM 设置未配置")
	}
	network, err := s.settings.GetNetworkSettings(ctx)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	client, err := newChatClient(settings, network)
	if err != nil {
		return ConnectionTestResult{}, err
	}
	response, err := client.complete(ctx, []chatMessage{
		{Role: "system", Content: "You are a connection test endpoint. Reply with exactly OK."},
		{Role: "user", Content: "请只输出 OK"},
	})
	if err != nil {
		return ConnectionTestResult{}, err
	}
	response = strings.Trim(strings.TrimSpace(response), "\"'` \n\r\t")
	if !strings.EqualFold(response, "OK") {
		return ConnectionTestResult{}, fmt.Errorf("LLM 返回了非预期内容: %s", response)
	}
	return ConnectionTestResult{Response: "OK"}, nil
}

func (s *Service) nextTarget(ctx context.Context) (translationTarget, bool, error) {
	for _, fetch := range []func(context.Context) (translationTarget, bool, error){
		s.nextAnimeTitle,
		s.nextAnimeSummary,
		s.nextEpisodeTitle,
		s.nextEpisodeSummary,
		s.nextCharacterSummary,
		s.nextActorShortSummary,
	} {
		target, ok, err := fetch(ctx)
		if err != nil || ok {
			return target, ok, err
		}
	}
	return translationTarget{}, false, nil
}

func (s *Service) nextAnimeTitle(ctx context.Context) (translationTarget, bool, error) {
	var target translationTarget
	err := s.db.QueryRowContext(ctx, `
SELECT bangumi_id, name
FROM anime_metadata
WHERE deleted_at IS NULL AND name != '' AND name_cn = ''
ORDER BY created_at, id
LIMIT 1`).Scan(&target.BangumiID, &target.SourceText)
	if errors.Is(err, sql.ErrNoRows) {
		return translationTarget{}, false, nil
	}
	if err != nil {
		return translationTarget{}, false, err
	}
	target.ID = target.BangumiID
	target.Kind = targetAnimeTitle
	target.AnimeTitle = target.SourceText
	target.TextType = "番剧标题"
	return target, true, nil
}

func (s *Service) nextAnimeSummary(ctx context.Context) (translationTarget, bool, error) {
	var target translationTarget
	err := s.db.QueryRowContext(ctx, `
SELECT bangumi_id, COALESCE(NULLIF(name_cn, ''), name), summary
FROM anime_metadata
WHERE deleted_at IS NULL AND summary != '' AND summary_cn = ''
ORDER BY created_at, id
LIMIT 1`).Scan(&target.BangumiID, &target.AnimeTitle, &target.SourceText)
	if errors.Is(err, sql.ErrNoRows) {
		return translationTarget{}, false, nil
	}
	if err != nil {
		return translationTarget{}, false, err
	}
	target.ID = target.BangumiID
	target.Kind = targetAnimeSummary
	target.TextType = "番剧剧情简介"
	return target, true, nil
}

func (s *Service) nextEpisodeTitle(ctx context.Context) (translationTarget, bool, error) {
	var target translationTarget
	err := s.db.QueryRowContext(ctx, `
SELECT e.id, e.bangumi_id, COALESCE(NULLIF(am.name_cn, ''), am.name), e.name
FROM anime_episodes e
JOIN anime_metadata am ON am.bangumi_id = e.bangumi_id
WHERE am.deleted_at IS NULL AND e.name != '' AND e.name_cn = ''
ORDER BY am.created_at, e.bangumi_id, e.sort_number, e.episode_id
LIMIT 1`).Scan(&target.ID, &target.BangumiID, &target.AnimeTitle, &target.SourceText)
	if errors.Is(err, sql.ErrNoRows) {
		return translationTarget{}, false, nil
	}
	if err != nil {
		return translationTarget{}, false, err
	}
	target.Kind = targetEpisodeTitle
	target.TextType = "分集标题"
	return target, true, nil
}

func (s *Service) nextEpisodeSummary(ctx context.Context) (translationTarget, bool, error) {
	var target translationTarget
	err := s.db.QueryRowContext(ctx, `
SELECT e.id, e.bangumi_id, COALESCE(NULLIF(am.name_cn, ''), am.name), e.description
FROM anime_episodes e
JOIN anime_metadata am ON am.bangumi_id = e.bangumi_id
WHERE am.deleted_at IS NULL AND e.description != '' AND e.description_cn = ''
ORDER BY am.created_at, e.bangumi_id, e.sort_number, e.episode_id
LIMIT 1`).Scan(&target.ID, &target.BangumiID, &target.AnimeTitle, &target.SourceText)
	if errors.Is(err, sql.ErrNoRows) {
		return translationTarget{}, false, nil
	}
	if err != nil {
		return translationTarget{}, false, err
	}
	target.Kind = targetEpisodeSummary
	target.TextType = "分集剧情简介"
	return target, true, nil
}

func (s *Service) nextCharacterSummary(ctx context.Context) (translationTarget, bool, error) {
	var target translationTarget
	err := s.db.QueryRowContext(ctx, `
SELECT c.id, c.bangumi_id, COALESCE(NULLIF(am.name_cn, ''), am.name), c.summary
FROM anime_characters c
JOIN anime_metadata am ON am.bangumi_id = c.bangumi_id
WHERE am.deleted_at IS NULL AND c.summary != '' AND c.summary_cn = ''
ORDER BY am.created_at, c.bangumi_id, c.id
LIMIT 1`).Scan(&target.ID, &target.BangumiID, &target.AnimeTitle, &target.SourceText)
	if errors.Is(err, sql.ErrNoRows) {
		return translationTarget{}, false, nil
	}
	if err != nil {
		return translationTarget{}, false, err
	}
	target.Kind = targetCharacterSummary
	target.TextType = "角色简介"
	return target, true, nil
}

func (s *Service) nextActorShortSummary(ctx context.Context) (translationTarget, bool, error) {
	var target translationTarget
	err := s.db.QueryRowContext(ctx, `
SELECT a.actor_id,
       COALESCE((
           SELECT COALESCE(NULLIF(am.name_cn, ''), am.name)
           FROM character_actors ca
           JOIN anime_metadata am ON am.bangumi_id = ca.bangumi_id
           WHERE ca.actor_id = a.actor_id AND am.deleted_at IS NULL
           ORDER BY am.created_at
           LIMIT 1
       ), ''),
       a.short_summary
FROM actors a
WHERE a.short_summary != '' AND a.short_summary_cn = ''
ORDER BY a.actor_id
LIMIT 1`).Scan(&target.ID, &target.AnimeTitle, &target.SourceText)
	if errors.Is(err, sql.ErrNoRows) {
		return translationTarget{}, false, nil
	}
	if err != nil {
		return translationTarget{}, false, err
	}
	target.Kind = targetActorShortSummary
	target.TextType = "声优简介"
	return target, true, nil
}

func (s *Service) saveTranslation(ctx context.Context, target translationTarget, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("翻译结果为空")
	}
	now := s.now().UTC().Unix()
	switch target.Kind {
	case targetAnimeTitle:
		_, err := s.db.ExecContext(ctx, "UPDATE anime_metadata SET name_cn = ? WHERE bangumi_id = ?", text, target.BangumiID)
		return err
	case targetAnimeSummary:
		_, err := s.db.ExecContext(ctx, "UPDATE anime_metadata SET summary_cn = ? WHERE bangumi_id = ?", text, target.BangumiID)
		return err
	case targetEpisodeTitle:
		_, err := s.db.ExecContext(ctx, "UPDATE anime_episodes SET name_cn = ?, updated_at = ? WHERE id = ?", text, now, target.ID)
		return err
	case targetEpisodeSummary:
		_, err := s.db.ExecContext(ctx, "UPDATE anime_episodes SET description_cn = ?, updated_at = ? WHERE id = ?", text, now, target.ID)
		return err
	case targetCharacterSummary:
		_, err := s.db.ExecContext(ctx, "UPDATE anime_characters SET summary_cn = ?, updated_at = ? WHERE id = ?", text, now, target.ID)
		return err
	case targetActorShortSummary:
		_, err := s.db.ExecContext(ctx, "UPDATE actors SET short_summary_cn = ?, updated_at = ? WHERE actor_id = ?", text, now, target.ID)
		return err
	default:
		return errors.New("未知翻译目标")
	}
}

func (c *chatClient) translate(ctx context.Context, target translationTarget) (string, error) {
	content, err := c.complete(ctx, []chatMessage{
		{Role: "system", Content: "你是严谨的日文、英文到简体中文的动画资料翻译助手。"},
		{Role: "user", Content: translationPrompt(target)},
	})
	if err != nil {
		return "", err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return "", errors.New("LLM 返回空内容")
	}
	return content, nil
}

func translationPrompt(target translationTarget) string {
	var builder strings.Builder
	builder.WriteString("请将下面文本翻译为简体中文。\n\n")
	if strings.TrimSpace(target.AnimeTitle) != "" {
		builder.WriteString("番剧标题：")
		builder.WriteString(target.AnimeTitle)
		builder.WriteString("\n")
	}
	builder.WriteString("文本类型：")
	builder.WriteString(target.TextType)
	builder.WriteString("\n\n要求：\n")
	builder.WriteString("1. 直接输出翻译内容，不要输出“好的”、说明、注释、引号、Markdown 代码块或其他无关信息。\n")
	builder.WriteString("2. 如果原文本身已经是中文，并且符合格式要求，请使用中文原样输出。\n")
	builder.WriteString("3. 保留或优化段落格式，允许为中文阅读习惯调整换行，但不要添加原文不存在的剧情信息。\n")
	ruleIndex := 4
	if target.Kind == targetAnimeSummary || target.Kind == targetEpisodeSummary || target.Kind == targetCharacterSummary {
		builder.WriteString(fmt.Sprintf("%d. 翻译剧情简介、分集简介或角色简介时，如果涉及角色名或专有名词，请在首次出现时用括号额外标注原文，例如：好实祈（好実いのり）。\n", ruleIndex))
		ruleIndex++
	}
	builder.WriteString(fmt.Sprintf("%d. 所有双引号统一使用中文直角引号「」。\n", ruleIndex))
	ruleIndex++
	if target.Kind == targetAnimeSummary || target.Kind == targetEpisodeSummary {
		builder.WriteString(fmt.Sprintf("%d. 如果原文包含非剧情简介内容，例如脚本、絵コンテ、演出、作画监督、制作人员或职员信息，请去除这些内容，不要翻译。\n", ruleIndex))
	}
	builder.WriteString("\n原文：\n")
	builder.WriteString(target.SourceText)
	return builder.String()
}

func isPlainChinese(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	hasHan := false
	for _, r := range text {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			return false
		}
		if unicode.IsLetter(r) {
			if unicode.Is(unicode.Han, r) {
				hasHan = true
				continue
			}
			return false
		}
	}
	return hasHan
}

func canCopyWithoutLLM(target translationTarget) bool {
	if !isPlainChinese(target.SourceText) {
		return false
	}
	if (target.Kind == targetAnimeSummary || target.Kind == targetEpisodeSummary) && containsStaffInfo(target.SourceText) {
		return false
	}
	return true
}

func containsStaffInfo(text string) bool {
	for _, keyword := range []string{
		"脚本", "劇本", "剧本", "絵コンテ", "分镜", "演出", "作画", "作畫",
		"総作画監督", "总作画监督", "監督", "监督", "製作", "制作", "Storyboard",
	} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func newChatClient(settings system.LLMSettings, network system.NetworkSettings) (*chatClient, error) {
	if strings.TrimSpace(settings.BaseURL) == "" || strings.TrimSpace(settings.Model) == "" {
		return nil, errors.New("LLM Base URL 和模型名称不能为空")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	proxyURL := strings.TrimSpace(network.HTTPSProxy)
	if proxyURL == "" {
		proxyURL = strings.TrimSpace(network.HTTPProxy)
	}
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("解析代理地址失败: %w", err)
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	return &chatClient{
		settings: settings,
		client:   &http.Client{Transport: transport, Timeout: 60 * time.Second},
	}, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *chatClient) complete(ctx context.Context, messages []chatMessage) (string, error) {
	payload, err := json.Marshal(chatRequest{
		Model:       strings.TrimSpace(c.settings.Model),
		Messages:    messages,
		Temperature: 0,
	})
	if err != nil {
		return "", err
	}
	endpoint := chatCompletionsEndpoint(c.settings.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.settings.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.settings.APIKey))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	var decoded chatResponse
	_ = json.Unmarshal(body, &decoded)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error != nil && strings.TrimSpace(decoded.Error.Message) != "" {
			return "", fmt.Errorf("LLM API 返回 %d: %s", resp.StatusCode, decoded.Error.Message)
		}
		return "", fmt.Errorf("LLM API 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if decoded.Error != nil && strings.TrimSpace(decoded.Error.Message) != "" {
		return "", errors.New(decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", errors.New("LLM API 未返回有效内容")
	}
	return decoded.Choices[0].Message.Content, nil
}

func chatCompletionsEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}
