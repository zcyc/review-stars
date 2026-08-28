package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The Vue build is embedded into the same binary for simple distribution.
//
//go:embed web/dist
var embeddedWeb embed.FS

const (
	defaultPort            = "8080"
	defaultGitHubAPIURL    = "https://api.github.com"
	defaultAIBaseURL       = "https://api.deepseek.com"
	defaultAIModel         = "deepseek-v4-flash"
	defaultAIMaxTokens     = 16000
	defaultLanguage        = "zh-CN"
	defaultReviewBatchSize = 50
	githubAPIVersion       = "2022-11-28"
)

type Config struct {
	Host             string
	Port             string
	GitHubToken      string
	GitHubAPIURL     string
	AIAPIKey         string
	AIBaseURL        string
	AIModel          string
	AIThinking       string
	AIMaxTokens      int
	Language         string
	TelegramBotToken string
	TelegramChatID   string
	DatabaseFile     string
	ReviewBatchSize  int
	ReviewCron       string
	ReviewCount      int
	RuleStatuses     string
	RuleStaleDays    int
	RuleMaxStars     int
}

func loadConfig() Config {
	return Config{
		Host:             envOr("HOST", "127.0.0.1"),
		Port:             envOr("PORT", defaultPort),
		GitHubToken:      strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		GitHubAPIURL:     strings.TrimRight(envOr("GITHUB_API_URL", defaultGitHubAPIURL), "/"),
		AIAPIKey:         strings.TrimSpace(os.Getenv("AI_API_KEY")),
		AIBaseURL:        strings.TrimRight(envOr("AI_BASE_URL", defaultAIBaseURL), "/"),
		AIModel:          envOr("AI_MODEL", defaultAIModel),
		AIThinking:       envAIThinking(),
		AIMaxTokens:      envInt("AI_MAX_TOKENS", defaultAIMaxTokens),
		Language:         envLanguage(),
		TelegramBotToken: strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramChatID:   strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")),
		DatabaseFile:     envOr("DATABASE_FILE", "review-stars.db"),
		ReviewBatchSize:  envInt("REVIEW_BATCH_SIZE", defaultReviewBatchSize),
		ReviewCron:       strings.TrimSpace(os.Getenv("REVIEW_CRON")),
		ReviewCount:      envInt("REVIEW_COUNT", 1),
		RuleStatuses:     strings.TrimSpace(envOr("RULE_STATUSES", "archived")),
		RuleStaleDays:    envIntAllowZero("RULE_STALE_DAYS", 180),
		RuleMaxStars:     envIntAllowZero("RULE_MAX_STARS", 1000),
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func envIntAllowZero(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func envAIThinking() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("AI_THINKING")), "enabled") {
		return "enabled"
	}
	return "disabled"
}

func envLanguage() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_LANGUAGE"))) {
	case "en", "en-us", "en_us":
		return "en"
	default:
		return defaultLanguage
	}
}

func logExternalRequest(service, method, target string) {
	log.Printf("[%s] request method=%s url=%s", service, method, redactExternalURL(target))
}

func logExternalResponse(service, method, target string, status int, started time.Time, body []byte) {
	responseBody := string(body)
	if strings.TrimSpace(responseBody) == "" {
		responseBody = "<empty>"
	}
	log.Printf("[%s] response method=%s url=%s status=%d duration=%s bytes=%d body=%s", service, method, redactExternalURL(target), status, time.Since(started).Round(time.Millisecond), len(body), responseBody)
}

func logExternalError(service, method, target string, started time.Time, err error) {
	log.Printf("[%s] response method=%s url=%s error=%v duration=%s", service, method, redactExternalURL(target), err, time.Since(started).Round(time.Millisecond))
}

func redactExternalURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host != "api.telegram.org" {
		return raw
	}
	parts := strings.SplitN(strings.TrimPrefix(parsed.Path, "/"), "/", 2)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "bot") {
		return raw
	}
	parsed.Path = "/bot-REDACTED"
	if len(parts) == 2 {
		parsed.Path += "/" + parts[1]
	}
	return parsed.String()
}

// loadDotEnv loads a local .env file without overwriting variables that were
// already exported by the shell. This keeps deployment environment variables
// higher priority while making `go run .` convenient for local use.
func loadDotEnv(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		separator := strings.IndexByte(line, '=')
		if separator < 1 {
			continue
		}
		key := strings.TrimSpace(line[:separator])
		if key == "" {
			continue
		}
		if existing, exists := os.LookupEnv(key); exists && strings.TrimSpace(existing) != "" {
			continue
		}
		value := strings.TrimSpace(line[separator+1:])
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			if decoded, decodeErr := strconv.Unquote(value); decodeErr == nil {
				value = decoded
			}
		} else if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		} else if comment := strings.Index(value, " #"); comment >= 0 {
			value = strings.TrimSpace(value[:comment])
		}
		_ = os.Setenv(key, value)
	}
}

type GitHubClient struct {
	baseURL string
	token   string
	http    *http.Client
}

type githubAPIError struct {
	status     int
	statusText string
	body       string
}

func (e *githubAPIError) Error() string {
	return fmt.Sprintf("github API %s: %s", e.statusText, strings.TrimSpace(e.body))
}

func (e *githubAPIError) isStarPermissionError() bool {
	return e.status == http.StatusForbidden && strings.Contains(e.body, "Resource not accessible by personal access token")
}

func (c *GitHubClient) request(ctx context.Context, method, endpoint string, result any) error {
	body, err := c.requestBytes(ctx, method, endpoint)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(body, result)
}

func (c *GitHubClient) requestBytes(ctx context.Context, method, endpoint string) ([]byte, error) {
	started := time.Now()
	target := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		logExternalError("github", method, target, started, err)
		return nil, err
	}
	if method == http.MethodGet && strings.HasPrefix(endpoint, "/user/starred?") {
		// The star media type adds starred_at to each repository item.
		req.Header.Set("Accept", "application/vnd.github.star+json")
	} else {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	logExternalRequest("github", method, target)
	resp, err := c.http.Do(req)
	if err != nil {
		logExternalError("github", method, target, started, err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		logExternalResponse("github", method, target, resp.StatusCode, started, body)
		return nil, &githubAPIError{status: resp.StatusCode, statusText: resp.Status, body: string(body)}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logExternalError("github", method, target, started, err)
		return nil, err
	}
	logExternalResponse("github", method, target, resp.StatusCode, started, body)
	return body, nil
}

type Repository struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	HTMLURL         string    `json:"html_url"`
	Description     string    `json:"description"`
	Language        string    `json:"language"`
	Topics          []string  `json:"topics"`
	Archived        bool      `json:"archived"`
	Disabled        bool      `json:"disabled"`
	Fork            bool      `json:"fork"`
	Private         bool      `json:"private"`
	StargazersCount int       `json:"stargazers_count"`
	ForksCount      int       `json:"forks_count"`
	OpenIssuesCount int       `json:"open_issues_count"`
	DefaultBranch   string    `json:"default_branch"`
	PushedAt        time.Time `json:"pushed_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	CreatedAt       time.Time `json:"created_at"`
	Owner           struct {
		Login string `json:"login"`
	} `json:"owner"`
	License *struct {
		SPDXID string `json:"spdx_id"`
		Name   string `json:"name"`
	} `json:"license"`
	StarredAt *time.Time `json:"starred_at,omitempty"`
}

// GitHub may return starred repositories either as repository objects with a
// starred_at field or as {starred_at, repo:{...}} envelopes, depending on the
// media type/API version. Keep both shapes compatible.
type starredRepositoryResponse struct {
	Repository
	StarredAt *time.Time  `json:"starred_at"`
	Repo      *Repository `json:"repo"`
}

func decodeStarredRepositories(data []byte) ([]Repository, error) {
	var response []starredRepositoryResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	repos := make([]Repository, 0, len(response))
	for _, item := range response {
		repo := item.Repository
		if item.Repo != nil {
			repo = *item.Repo
		}
		if item.StarredAt != nil {
			repo.StarredAt = item.StarredAt
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

func (c *GitHubClient) ListStarred(ctx context.Context) ([]Repository, error) {
	if c.token == "" {
		return nil, errors.New("GITHUB_TOKEN is not configured")
	}
	all := make([]Repository, 0)
	for page := 1; ; page++ {
		endpoint := fmt.Sprintf("/user/starred?per_page=100&page=%d&sort=updated&direction=desc", page)
		body, err := c.requestBytes(ctx, http.MethodGet, endpoint)
		if err != nil {
			return nil, err
		}
		repos, err := decodeStarredRepositories(body)
		if err != nil {
			return nil, fmt.Errorf("decode github starred repositories: %w", err)
		}
		all = append(all, repos...)
		if len(repos) < 100 {
			break
		}
	}
	return all, nil
}

func (c *GitHubClient) Unstar(ctx context.Context, fullName string) error {
	parts := strings.Split(strings.Trim(fullName, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid repository name %q", fullName)
	}
	endpoint := "/user/starred/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1])
	return c.request(ctx, http.MethodDelete, endpoint, nil)
}

type AIClient struct {
	baseURL   string
	key       string
	model     string
	thinking  string
	maxTokens int
	language  string
	http      *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string              `json:"model"`
	Messages       []chatMessage       `json:"messages"`
	Temperature    float64             `json:"temperature,omitempty"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	Thinking       *chatThinking       `json:"thinking,omitempty"`
	ResponseFormat *chatResponseFormat `json:"response_format,omitempty"`
}

type chatThinking struct {
	Type string `json:"type"`
}

type chatResponseFormat struct {
	Type string `json:"type"`
}

type promptTokenDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type AIUsage struct {
	PromptTokens         int                 `json:"prompt_tokens"`
	CompletionTokens     int                 `json:"completion_tokens"`
	TotalTokens          int                 `json:"total_tokens"`
	PromptCacheHitTokens int                 `json:"prompt_cache_hit_tokens"`
	PromptTokensDetails  *promptTokenDetails `json:"prompt_tokens_details,omitempty"`
}

func (usage AIUsage) normalized() AIUsage {
	if usage.PromptCacheHitTokens == 0 && usage.PromptTokensDetails != nil {
		usage.PromptCacheHitTokens = usage.PromptTokensDetails.CachedTokens
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}

func (usage *AIUsage) add(other AIUsage) {
	usage.PromptTokens += other.PromptTokens
	usage.CompletionTokens += other.CompletionTokens
	usage.TotalTokens += other.TotalTokens
	usage.PromptCacheHitTokens += other.PromptCacheHitTokens
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage AIUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type AIReview struct {
	FullName string   `json:"full_name"`
	Decision string   `json:"decision"`
	Score    int      `json:"score"`
	Summary  string   `json:"summary"`
	Reasons  []string `json:"reasons"`
}

func (c *AIClient) Review(ctx context.Context, repos []Repository) ([]AIReview, AIUsage, error) {
	if c.key == "" {
		return nil, AIUsage{}, errors.New("AI_API_KEY is not configured")
	}
	input := make([]map[string]any, 0, len(repos))
	for _, repo := range repos {
		topics := repo.Topics
		if len(topics) > 10 {
			topics = topics[:10]
		}
		input = append(input, map[string]any{
			"full_name":   repo.FullName,
			"description": truncateRunes(repo.Description, 200),
			"language":    repo.Language,
			"topics":      topics,
			"archived":    repo.Archived,
			"disabled":    repo.Disabled,
			"fork":        repo.Fork,
			"stars":       repo.StargazersCount,
			"pushed_at":   repo.PushedAt.UTC().Format(time.RFC3339),
			"starred_at":  starredAt(repo),
		})
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, AIUsage{}, err
	}
	languageInstruction := "summary 不超过 15 个字，reasons 最多 2 条且每条简短；summary 和 reasons 必须使用中文。"
	if c.language == "en" {
		languageInstruction = "Keep summary under 15 words and return at most 2 concise reasons. Write summary and reasons in English."
	}
	prompt := `你是一个 GitHub Stars 整理助手。请根据仓库元数据判断用户是否还值得保留这个 Star。

评判原则：已归档、已禁用、长期没有更新、明显是一次性实验、重复/低价值 fork，优先建议取消；活跃、基础设施、学习资料、值得持续关注的项目应保留；信息不足时标记 review，不要武断。

	只返回一个 JSON 对象，不要 Markdown、不要解释。对象必须包含 reviews 数组，数组中的每一项必须包含：
{"reviews":[{"full_name":"owner/name","decision":"unstar|keep|review","score":0,"summary":"one-sentence summary","reasons":["reason"]}]}

score 是 0-100 的取消建议置信度。必须为输入中的每个仓库返回一项，并保持 full_name 不变。

` + languageInstruction + `

仓库数据：` + string(data)
	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: "你只输出合法 JSON。"},
			{Role: "user", Content: prompt},
		},
		Temperature:    0.2,
		MaxTokens:      c.maxTokens,
		Thinking:       &chatThinking{Type: c.thinking},
		ResponseFormat: aiReviewResponseFormat(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, AIUsage{}, err
	}
	target := chatCompletionsURL(c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, AIUsage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	logExternalRequest("ai", http.MethodPost, target)
	started := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		logExternalError("ai", http.MethodPost, target, started, err)
		return nil, AIUsage{}, err
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	logExternalResponse("ai", http.MethodPost, target, resp.StatusCode, started, responseBody)
	if readErr != nil {
		return nil, AIUsage{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, AIUsage{}, fmt.Errorf("AI API %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}
	var completion chatResponse
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		return nil, AIUsage{}, fmt.Errorf("decode AI response: %w", err)
	}
	usage := completion.Usage.normalized()
	if completion.Error != nil {
		return nil, usage, errors.New(completion.Error.Message)
	}
	if len(completion.Choices) == 0 {
		return nil, usage, errors.New("AI API returned no choices")
	}
	reviews, err := parseAIReviews(completion.Choices[0].Message.Content)
	return reviews, usage, err
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func chatCompletionsURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func starredAt(repo Repository) string {
	if repo.StarredAt == nil {
		return ""
	}
	return repo.StarredAt.UTC().Format(time.RFC3339)
}

func parseAIReviews(content string) ([]AIReview, error) {
	content = strings.TrimSpace(content)
	candidates := []string{content}
	if block := extractJSONBlock(content); block != "" && block != content {
		candidates = append(candidates, block)
	}
	var lastErr error
	for _, candidate := range candidates {
		var reviews []AIReview
		if err := json.Unmarshal([]byte(candidate), &reviews); err == nil {
			return normalizeAIReviews(reviews), nil
		} else {
			lastErr = err
		}

		var envelope struct {
			Reviews []AIReview `json:"reviews"`
		}
		if err := json.Unmarshal([]byte(candidate), &envelope); err == nil && envelope.Reviews != nil {
			return normalizeAIReviews(envelope.Reviews), nil
		} else if err != nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("AI response did not contain JSON")
	}
	preview := content
	if len(preview) > 300 {
		preview = preview[:300] + "…"
	}
	return nil, fmt.Errorf("decode AI JSON: %w (response: %q)", lastErr, preview)
}

func extractJSONBlock(content string) string {
	start := strings.IndexAny(content, "[{")
	if start < 0 {
		return ""
	}
	opening := content[start]
	closing := byte(']')
	if opening == '{' {
		closing = '}'
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(content); i++ {
		character := content[i]
		if inString {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		if character == '"' {
			inString = true
			continue
		}
		if character == opening {
			depth++
		} else if character == closing {
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}
	return ""
}

func normalizeAIReviews(reviews []AIReview) []AIReview {
	for i := range reviews {
		reviews[i] = normalizeReview(reviews[i])
	}
	return reviews
}

func aiReviewResponseFormat() *chatResponseFormat {
	return &chatResponseFormat{Type: "json_object"}
}

func normalizeReview(review AIReview) AIReview {
	review.Decision = strings.ToLower(strings.TrimSpace(review.Decision))
	if review.Decision != "unstar" && review.Decision != "keep" && review.Decision != "review" {
		review.Decision = "review"
	}
	if review.Score < 0 {
		review.Score = 0
	}
	if review.Score > 100 {
		review.Score = 100
	}
	if len(review.Reasons) == 0 && review.Summary != "" {
		review.Reasons = []string{review.Summary}
	}
	return review
}

type RepositoryReview struct {
	Repository
	Decision       string   `json:"decision"`
	Score          int      `json:"score"`
	Summary        string   `json:"summary"`
	Reasons        []string `json:"reasons"`
	Source         string   `json:"source"`
	AILanguage     string   `json:"ai_language,omitempty"`
	ReviewLanguage string   `json:"review_language,omitempty"`
}

func ruleReview(repo Repository, config Config, now time.Time) RepositoryReview {
	archivedReason := "仓库已归档"
	disabledReason := "仓库已被 GitHub 禁用"
	noMatchSummary := "未同时命中配置规则"
	noRulesSummary := "没有启用规则"
	matchedSummary := "同时命中 %d 条规则"
	if config.Language == "en" {
		archivedReason = "Repository is archived"
		disabledReason = "Repository is disabled by GitHub"
		noMatchSummary = "Did not match all configured rules"
		noRulesSummary = "No rules are enabled"
		matchedSummary = "Matched %d rules simultaneously"
	}
	matchedReasons := make([]string, 0, 3)
	statuses := make(map[string]bool)
	for _, status := range strings.Split(config.RuleStatuses, ",") {
		status = strings.ToLower(strings.TrimSpace(status))
		if status != "" && status != "none" {
			statuses[status] = true
		}
	}
	enabledRules := 0
	matchedRules := 0
	if len(statuses) > 0 {
		enabledRules++
		statusMatched := (statuses["archived"] && repo.Archived) || (statuses["disabled"] && repo.Disabled)
		if statusMatched {
			matchedRules++
			statusReasons := make([]string, 0, 2)
			if statuses["archived"] && repo.Archived {
				statusReasons = append(statusReasons, archivedReason)
			}
			if statuses["disabled"] && repo.Disabled {
				statusReasons = append(statusReasons, disabledReason)
			}
			matchedReasons = append(matchedReasons, statusReasons...)
		}
	}
	lastActivity := repo.PushedAt
	if lastActivity.IsZero() {
		lastActivity = repo.UpdatedAt
	}
	if config.RuleStaleDays > 0 {
		enabledRules++
		if !lastActivity.IsZero() && !lastActivity.After(now) && now.Sub(lastActivity) >= time.Duration(config.RuleStaleDays)*24*time.Hour {
			matchedRules++
			if config.Language == "en" {
				matchedReasons = append(matchedReasons, fmt.Sprintf("Not updated for more than %d days", config.RuleStaleDays))
			} else {
				matchedReasons = append(matchedReasons, fmt.Sprintf("超过 %d 天未更新", config.RuleStaleDays))
			}
		}
	}
	if config.RuleMaxStars > 0 {
		enabledRules++
		if repo.StargazersCount < config.RuleMaxStars {
			matchedRules++
			if config.Language == "en" {
				matchedReasons = append(matchedReasons, fmt.Sprintf("Fewer than %d Stars (current: %d)", config.RuleMaxStars, repo.StargazersCount))
			} else {
				matchedReasons = append(matchedReasons, fmt.Sprintf("Star 少于 %d（当前 %d）", config.RuleMaxStars, repo.StargazersCount))
			}
		}
	}
	decision := "keep"
	score := 0
	reasons := []string(nil)
	summary := noMatchSummary
	if enabledRules == 0 {
		summary = noRulesSummary
	} else if matchedRules == enabledRules {
		decision = "unstar"
		score = 100
		reasons = matchedReasons
		summary = fmt.Sprintf(matchedSummary, enabledRules)
	}
	return RepositoryReview{
		Repository:     repo,
		Decision:       decision,
		Score:          score,
		Summary:        summary,
		Reasons:        reasons,
		Source:         "rule",
		ReviewLanguage: config.Language,
	}
}

func mergeAIReviews(repos []Repository, aiReviews []AIReview, language string) ([]RepositoryReview, error) {
	byName := make(map[string]AIReview, len(aiReviews))
	for _, review := range aiReviews {
		byName[strings.ToLower(review.FullName)] = normalizeReview(review)
	}
	result := make([]RepositoryReview, 0, len(repos))
	for _, repo := range repos {
		review, ok := byName[strings.ToLower(repo.FullName)]
		if !ok {
			return nil, fmt.Errorf("AI response did not include %s", repo.FullName)
		}
		result = append(result, RepositoryReview{
			Repository:     repo,
			Decision:       review.Decision,
			Score:          review.Score,
			Summary:        review.Summary,
			Reasons:        review.Reasons,
			Source:         "ai",
			AILanguage:     language,
			ReviewLanguage: language,
		})
	}
	return result, nil
}

type ReviewStats struct {
	Total  int `json:"total"`
	Unstar int `json:"unstar"`
	Review int `json:"review"`
	Keep   int `json:"keep"`
}

func statsFor(reviews []RepositoryReview) ReviewStats {
	stats := ReviewStats{Total: len(reviews)}
	for _, review := range reviews {
		switch review.Decision {
		case "unstar":
			stats.Unstar++
		case "review":
			stats.Review++
		default:
			stats.Keep++
		}
	}
	return stats
}

func statsForCollection(repos []Repository, reviews []RepositoryReview) ReviewStats {
	stats := statsFor(reviews)
	stats.Total = len(repos)
	if pending := len(repos) - len(reviews); pending > 0 {
		stats.Review += pending
	}
	return stats
}

func filterAIReviewsByLanguage(reviews []RepositoryReview, language string) []RepositoryReview {
	filtered := make([]RepositoryReview, 0, len(reviews))
	for _, review := range reviews {
		if review.Source == "ai" && review.AILanguage == language {
			filtered = append(filtered, review)
		}
	}
	return filtered
}

func applyRulePrefilter(repos []Repository, aiReviews []RepositoryReview, config Config, now time.Time) []RepositoryReview {
	aiByName := make(map[string]RepositoryReview, len(aiReviews))
	for _, review := range aiReviews {
		aiByName[cacheKey(review.FullName)] = review
	}
	result := make([]RepositoryReview, 0, len(repos))
	for _, repo := range repos {
		rule := ruleReview(repo, config, now)
		if rule.Decision == "unstar" {
			result = append(result, rule)
			continue
		}
		if review, ok := aiByName[cacheKey(repo.FullName)]; ok {
			result = append(result, review)
		}
	}
	return result
}

func aiReviewComplete(repos []Repository, reviews []RepositoryReview, language string) bool {
	if len(repos) == 0 || len(reviews) != len(repos) {
		return false
	}
	byName := make(map[string]RepositoryReview, len(reviews))
	for _, review := range reviews {
		byName[cacheKey(review.FullName)] = review
	}
	for _, repo := range repos {
		review, ok := byName[cacheKey(repo.FullName)]
		if !ok {
			return false
		}
		switch review.Source {
		case "ai":
			if review.AILanguage != language {
				return false
			}
		case "rule":
			if review.ReviewLanguage != language {
				return false
			}
		default:
			return false
		}
	}
	return true
}

type Store struct {
	mu        sync.RWMutex
	repos     []Repository
	reviews   []RepositoryReview
	updatedAt time.Time
}

func (s *Store) setData(repos []Repository, reviews []RepositoryReview) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repos = append([]Repository{}, repos...)
	s.reviews = append([]RepositoryReview{}, reviews...)
	s.updatedAt = time.Now()
}

func (s *Store) snapshot() ([]Repository, []RepositoryReview, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Repository{}, s.repos...), append([]RepositoryReview{}, s.reviews...), s.updatedAt
}

func (s *Store) remove(fullName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filteredRepos := s.repos[:0]
	for _, repo := range s.repos {
		if !strings.EqualFold(repo.FullName, fullName) {
			filteredRepos = append(filteredRepos, repo)
		}
	}
	s.repos = filteredRepos
	filteredReviews := s.reviews[:0]
	for _, review := range s.reviews {
		if !strings.EqualFold(review.FullName, fullName) {
			filteredReviews = append(filteredReviews, review)
		}
	}
	s.reviews = filteredReviews
}

func cacheKey(fullName string) string {
	return strings.ToLower(strings.TrimSpace(fullName))
}

func repositoryFingerprint(repo Repository) string {
	topics := repo.Topics
	if len(topics) > 10 {
		topics = topics[:10]
	}
	snapshot := struct {
		FullName    string   `json:"full_name"`
		Description string   `json:"description"`
		Language    string   `json:"language"`
		Topics      []string `json:"topics"`
		Archived    bool     `json:"archived"`
		Disabled    bool     `json:"disabled"`
		Fork        bool     `json:"fork"`
		PushedMonth string   `json:"pushed_month"`
	}{
		FullName: repo.FullName, Description: truncateRunes(repo.Description, 200), Language: repo.Language,
		Topics: topics, Archived: repo.Archived, Disabled: repo.Disabled, Fork: repo.Fork,
		PushedMonth: pushedMonth(repo.PushedAt),
	}
	data, _ := json.Marshal(snapshot)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func pushedMonth(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01")
}

type App struct {
	config   Config
	github   *GitHubClient
	ai       *AIClient
	telegram *TelegramClient
	store    *Store
	db       *SQLiteStore
	reviewMu sync.Mutex
}

type TelegramClient struct {
	botToken string
	chatID   string
	http     *http.Client
}

type telegramMessage struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

type telegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

func (c *TelegramClient) Send(ctx context.Context, repo RepositoryReview) error {
	if c.botToken == "" || c.chatID == "" {
		return errors.New("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required")
	}
	text := fmt.Sprintf("⭐ <b>Star 回顾提醒</b>\n\n<b>%s</b>\n%s\n\n<a href=\"%s\">打开 GitHub 仓库</a>",
		escapeHTML(repo.FullName), escapeHTML(repo.Summary), repo.HTMLURL)
	body, err := json.Marshal(telegramMessage{ChatID: c.chatID, Text: text, ParseMode: "HTML", DisableWebPagePreview: false})
	if err != nil {
		return err
	}
	telegramURL := "https://api.telegram.org/bot" + c.botToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, telegramURL, bytes.NewReader(body))
	if err != nil {
		logExternalError("telegram", http.MethodPost, telegramURL, time.Now(), err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	logExternalRequest("telegram", http.MethodPost, telegramURL)
	started := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		logExternalError("telegram", http.MethodPost, telegramURL, started, err)
		return err
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	logExternalResponse("telegram", http.MethodPost, telegramURL, resp.StatusCode, started, responseBody)
	if readErr != nil {
		return readErr
	}
	var result telegramResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !result.OK {
		return fmt.Errorf("telegram API %s: %s", resp.Status, result.Description)
	}
	return nil
}

func escapeHTML(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	return strings.ReplaceAll(value, "\"", "&quot;")
}

func (a *App) listStars(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	repos, _, updatedAt := a.store.snapshot()
	writeJSON(w, http.StatusOK, map[string]any{"repositories": repos, "count": len(repos), "updated_at": updatedAt})
}

func (a *App) syncAllStars(ctx context.Context) ([]Repository, []RepositoryReview, error) {
	repos, err := a.github.ListStarred(ctx)
	if err != nil {
		return nil, nil, err
	}
	if a.db == nil {
		return nil, nil, errors.New("SQLite database is not configured")
	}
	if err := a.db.SaveRepositories(repos); err != nil {
		return nil, nil, fmt.Errorf("save repositories to sqlite: %w", err)
	}
	reviews, err := a.db.ListReviews(repos)
	if err != nil {
		return nil, nil, err
	}
	reviews = filterAIReviewsByLanguage(reviews, a.config.Language)
	reviews = applyRulePrefilter(repos, reviews, a.config, time.Now())
	a.store.setData(repos, reviews)
	return repos, reviews, nil
}

func (a *App) reviewAll(ctx context.Context, force bool) ([]RepositoryReview, ReviewStats, int, int, int, int, []string, error) {
	a.reviewMu.Lock()
	defer a.reviewMu.Unlock()

	repos, _, _ := a.store.snapshot()
	if len(repos) == 0 {
		return nil, ReviewStats{}, 0, 0, 0, 0, nil, errors.New("还没有同步仓库，请先点击“同步仓库”")
	}
	if a.db == nil {
		return nil, ReviewStats{}, 0, 0, 0, 0, nil, errors.New("SQLite database is not configured")
	}
	byName := make(map[string]RepositoryReview, len(repos))
	uncached := make([]Repository, 0, len(repos))
	cachedCount := 0
	ruleMatchedCount := 0
	now := time.Now()
	for _, repo := range repos {
		rule := ruleReview(repo, a.config, now)
		if rule.Decision == "unstar" {
			byName[cacheKey(repo.FullName)] = rule
			ruleMatchedCount++
			continue
		}
		if !force {
			cached, ok, err := a.db.GetReview(repo)
			if err != nil {
				return nil, ReviewStats{}, 0, 0, 0, ruleMatchedCount, nil, err
			}
			if ok && cached.Source == "ai" && cached.AILanguage == a.config.Language {
				byName[cacheKey(repo.FullName)] = cached
				cachedCount++
				continue
			}
		}
		uncached = append(uncached, repo)
	}

	batchSize := a.config.ReviewBatchSize
	if batchSize < 1 {
		batchSize = defaultReviewBatchSize
	}
	batchCount := 0
	aiReviewedCount := 0
	usage := AIUsage{}
	warnings := make([]string, 0)
	totalBatches := (len(uncached) + batchSize - 1) / batchSize
	log.Printf("[review] start repositories=%d rule_matched=%d ai_uncached=%d ai_cached=%d batch_size=%d batches=%d", len(repos), ruleMatchedCount, len(uncached), cachedCount, batchSize, totalBatches)
	for start := 0; start < len(uncached); start += batchSize {
		end := start + batchSize
		if end > len(uncached) {
			end = len(uncached)
		}
		batch := uncached[start:end]
		batchCount++
		log.Printf("[ai] review batch=%d/%d start repositories=%d", batchCount, totalBatches, len(batch))
		if a.ai == nil {
			return nil, ReviewStats{}, cachedCount, aiReviewedCount, batchCount, ruleMatchedCount, warnings, errors.New("AI_API_KEY is not configured")
		}
		aiReviews, batchUsage, err := a.ai.Review(ctx, batch)
		usage.add(batchUsage)
		if err != nil {
			return nil, ReviewStats{}, cachedCount, aiReviewedCount, batchCount, ruleMatchedCount, warnings, fmt.Errorf("AI review failed in batch %d: %w", batchCount, err)
		}
		merged, err := mergeAIReviews(batch, aiReviews, a.config.Language)
		if err != nil {
			return nil, ReviewStats{}, cachedCount, aiReviewedCount, batchCount, ruleMatchedCount, warnings, fmt.Errorf("AI review result invalid in batch %d: %w", batchCount, err)
		}
		for _, review := range merged {
			byName[cacheKey(review.FullName)] = review
			if err := a.db.SaveReview(review); err != nil {
				return nil, ReviewStats{}, cachedCount, aiReviewedCount, batchCount, ruleMatchedCount, warnings, fmt.Errorf("save AI review to sqlite: %w", err)
			}
			aiReviewedCount++
		}
		log.Printf("[ai] review batch=%d/%d complete reviews=%d prompt_tokens=%d completion_tokens=%d prompt_cache_hit_tokens=%d total_tokens=%d", batchCount, totalBatches, len(merged), batchUsage.PromptTokens, batchUsage.CompletionTokens, batchUsage.PromptCacheHitTokens, batchUsage.TotalTokens)
	}

	reviews := make([]RepositoryReview, 0, len(repos))
	for _, repo := range repos {
		if review, ok := byName[cacheKey(repo.FullName)]; ok {
			reviews = append(reviews, review)
		} else {
			return nil, ReviewStats{}, cachedCount, aiReviewedCount, batchCount, ruleMatchedCount, warnings, fmt.Errorf("review did not produce a result for %s", repo.FullName)
		}
	}
	a.store.setData(repos, reviews)
	log.Printf("[review] complete repositories=%d rule_matched=%d ai_cached=%d ai_reviewed=%d batches=%d prompt_tokens=%d completion_tokens=%d prompt_cache_hit_tokens=%d total_tokens=%d", len(reviews), ruleMatchedCount, cachedCount, aiReviewedCount, batchCount, usage.PromptTokens, usage.CompletionTokens, usage.PromptCacheHitTokens, usage.TotalTokens)
	return reviews, statsFor(reviews), cachedCount, aiReviewedCount, batchCount, ruleMatchedCount, warnings, nil
}

func (a *App) sync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	repos, reviews, err := a.syncAllStars(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repositories": repos,
		"reviews":      reviews,
		"ai_complete":  aiReviewComplete(repos, reviews, a.config.Language),
		"count":        len(repos),
		"stats":        statsForCollection(repos, reviews),
		"updated_at":   time.Now(),
	})
}

func (a *App) review(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		repos, reviews, updatedAt := a.store.snapshot()
		if len(reviews) == 0 {
			writeError(w, http.StatusNotFound, errors.New("还没有评审结果，请先同步仓库并开始评审"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"reviews": reviews, "stats": statsForCollection(repos, reviews), "ai_complete": aiReviewComplete(repos, reviews, a.config.Language), "updated_at": updatedAt})
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	force := r.URL.Query().Get("force") == "1" || strings.EqualFold(r.URL.Query().Get("force"), "true")
	continueOnly := r.URL.Query().Get("continue") == "1" || strings.EqualFold(r.URL.Query().Get("continue"), "true")
	if continueOnly {
		force = false
		repos, reviews, _ := a.store.snapshot()
		if aiReviewComplete(repos, reviews, a.config.Language) {
			writeError(w, http.StatusConflict, errors.New("AI review is already complete"))
			return
		}
	}
	reviews, stats, cachedCount, aiReviewedCount, batchCount, ruleMatchedCount, warnings, err := a.reviewAll(r.Context(), force)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	repos, _, _ := a.store.snapshot()
	response := map[string]any{
		"reviews":            reviews,
		"stats":              stats,
		"cached_count":       cachedCount,
		"ai_reviewed_count":  aiReviewedCount,
		"batch_count":        batchCount,
		"rule_matched_count": ruleMatchedCount,
		"ai_complete":        aiReviewComplete(repos, reviews, a.config.Language),
		"updated_at":         time.Now(),
	}
	if len(warnings) > 0 {
		response["warning"] = strings.Join(warnings, "\n")
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) randomRepository(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	_, reviews, _ := a.store.snapshot()
	if len(reviews) > 0 {
		count := 1
		if rawCount := strings.TrimSpace(r.URL.Query().Get("count")); rawCount != "" {
			if parsed, err := strconv.Atoi(rawCount); err == nil {
				count = parsed
			}
		}
		selected := randomReviewBatch(reviews, count)
		writeJSON(w, http.StatusOK, map[string]any{"repositories": selected, "count": len(selected)})
		return
	}
	writeError(w, http.StatusNotFound, errors.New("还没有评审结果，请先开始 AI 评审"))
}

func (a *App) unstar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	fullName := strings.TrimPrefix(r.URL.Path, "/api/stars/")
	fullName, _ = url.PathUnescape(fullName)
	repos, _, _ := a.store.snapshot()
	for _, repo := range repos {
		if repo.FullName == fullName && repo.Archived {
			starsURL := githubStarsURL(fullName)
			writeJSON(w, http.StatusConflict, map[string]any{"error": "archived repositories must be unstarred from GitHub Stars", "stars_url": starsURL})
			return
		}
	}
	if err := a.github.Unstar(r.Context(), fullName); err != nil {
		var githubErr *githubAPIError
		if errors.As(err, &githubErr) && githubErr.isStarPermissionError() {
			starsURL := githubStarsURL(fullName)
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":     "GitHub token needs Starring: read and write plus Metadata: read to remove Stars",
				"stars_url": starsURL,
			})
			return
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}
	a.store.remove(fullName)
	if a.db != nil {
		if err := a.db.DeleteRepository(fullName); err != nil {
			log.Printf("warning: could not delete repository from sqlite: %v", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": fullName})
}

func githubStarsURL(fullName string) string {
	return "https://github.com/stars?q=" + url.QueryEscape(fullName)
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"github_configured":   a.config.GitHubToken != "",
		"language":            a.config.Language,
		"ai_configured":       a.config.AIAPIKey != "",
		"telegram_configured": a.config.TelegramBotToken != "" && a.config.TelegramChatID != "",
		"ai_model":            a.config.AIModel,
		"ai_thinking":         a.config.AIThinking,
		"ai_max_tokens":       a.config.AIMaxTokens,
		"database_file":       a.config.DatabaseFile,
		"review_cron":         a.config.ReviewCron,
		"review_count":        a.config.ReviewCount,
		"rule_statuses":       a.config.RuleStatuses,
		"rule_stale_days":     a.config.RuleStaleDays,
		"rule_max_stars":      a.config.RuleMaxStars,
	})
}

func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", a.health)
	mux.HandleFunc("/api/stars", a.listStars)
	mux.HandleFunc("/api/stars/", a.unstar)
	mux.HandleFunc("/api/sync", a.sync)
	mux.HandleFunc("/api/review", a.review)
	mux.HandleFunc("/api/random", a.randomRepository)
	mux.HandleFunc("/", serveSPA)
	return loggingMiddleware(mux)
}

func serveSPA(w http.ResponseWriter, r *http.Request) {
	requested := strings.TrimPrefix(pathpkg.Clean("/"+r.URL.Path), "/")
	if requested == "" || requested == "." {
		requested = "index.html"
	}
	filePath := pathpkg.Join("web/dist", requested)
	data, err := fs.ReadFile(embeddedWeb, filePath)
	contentType := mime.TypeByExtension(pathpkg.Ext(requested))
	if err != nil {
		data, err = fs.ReadFile(embeddedWeb, "web/dist/index.html")
		contentType = "text/html; charset=utf-8"
	}
	if err != nil {
		http.Error(w, "frontend build not found", http.StatusNotFound)
		return
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond))
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func main() {
	loadDotEnv(".env")
	config := loadConfig()
	httpClient := &http.Client{Timeout: 45 * time.Second}
	github := &GitHubClient{baseURL: config.GitHubAPIURL, token: config.GitHubToken, http: httpClient}
	ai := &AIClient{baseURL: config.AIBaseURL, key: config.AIAPIKey, model: config.AIModel, thinking: config.AIThinking, maxTokens: config.AIMaxTokens, language: config.Language, http: httpClient}
	telegram := &TelegramClient{botToken: config.TelegramBotToken, chatID: config.TelegramChatID, http: httpClient}
	database, err := openSQLiteStore(config.DatabaseFile)
	if err != nil {
		log.Fatalf("open sqlite database: %v", err)
	}
	defer database.Close()
	repos, err := database.ListRepositories()
	if err != nil {
		log.Fatalf("load repositories from sqlite: %v", err)
	}
	reviews, err := database.ListReviews(repos)
	if err != nil {
		log.Fatalf("load reviews from sqlite: %v", err)
	}
	reviews = filterAIReviewsByLanguage(reviews, config.Language)
	reviews = applyRulePrefilter(repos, reviews, config, time.Now())
	store := &Store{}
	store.setData(repos, reviews)
	app := &App{config: config, github: github, ai: ai, telegram: telegram, store: store, db: database}
	scheduler := startReminderScheduler(app)
	if scheduler != nil {
		defer scheduler.Stop()
	}

	server := &http.Server{
		Addr:              config.Host + ":" + config.Port,
		Handler:           app.handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("Review Stars listening on http://%s:%s (loaded repositories=%d reviews=%d database=%s)", config.Host, config.Port, len(repos), len(reviews), config.DatabaseFile)
	if config.GitHubToken == "" {
		log.Printf("warning: GITHUB_TOKEN is not configured")
	}
	if config.AIAPIKey == "" {
		log.Printf("warning: AI_API_KEY is not configured; AI review is unavailable")
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
