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
	"math/rand"
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
	defaultOpenRouterURL   = "https://openrouter.ai/api/v1/chat/completions"
	defaultOpenRouterModel = "openrouter/free"
	githubAPIVersion       = "2022-11-28"
)

type Config struct {
	Host             string
	Port             string
	GitHubToken      string
	GitHubAPIURL     string
	OpenRouterAPIKey string
	OpenRouterURL    string
	OpenRouterModel  string
	TelegramBotToken string
	TelegramChatID   string
	AppURL           string
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
		OpenRouterAPIKey: strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		OpenRouterURL:    envOr("OPENROUTER_URL", defaultOpenRouterURL),
		OpenRouterModel:  envOr("OPENROUTER_MODEL", defaultOpenRouterModel),
		TelegramBotToken: strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramChatID:   strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")),
		AppURL:           strings.TrimRight(os.Getenv("APP_URL"), "/"),
		DatabaseFile:     envOr("DATABASE_FILE", "review-stars.db"),
		ReviewBatchSize:  envInt("REVIEW_BATCH_SIZE", 20),
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

const maxExternalResponseLogBytes = 2000

func logExternalResponse(service, method, target string, status int, started time.Time, body []byte) {
	preview := strings.TrimSpace(string(body))
	preview = strings.Join(strings.Fields(preview), " ")
	if preview == "" {
		preview = "<empty>"
	}
	if len(preview) > maxExternalResponseLogBytes {
		preview = preview[:maxExternalResponseLogBytes] + "…"
	}
	log.Printf("[%s] response method=%s url=%s status=%d duration=%s bytes=%d body=%s", service, method, redactExternalURL(target), status, time.Since(started).Round(time.Millisecond), len(body), preview)
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
	// The star media type adds starred_at to each repository item.
	req.Header.Set("Accept", "application/vnd.github.star+json")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		logExternalError("github", method, target, started, err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		logExternalResponse("github", method, target, resp.StatusCode, started, body)
		return nil, fmt.Errorf("github API %s: %s", resp.Status, strings.TrimSpace(string(body)))
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

type OpenRouterClient struct {
	url    string
	key    string
	model  string
	http   *http.Client
	appURL string
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
	ResponseFormat *chatResponseFormat `json:"response_format,omitempty"`
}

type chatResponseFormat struct {
	Type       string         `json:"type"`
	JSONSchema chatJSONSchema `json:"json_schema"`
}

type chatJSONSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type AIReview struct {
	FullName   string   `json:"full_name"`
	Decision   string   `json:"decision"`
	Score      int      `json:"score"`
	Summary    string   `json:"summary"`
	Reasons    []string `json:"reasons"`
	NextAction string   `json:"next_action"`
}

func (c *OpenRouterClient) Review(ctx context.Context, repos []Repository) ([]AIReview, error) {
	if c.key == "" {
		return nil, errors.New("OPENROUTER_API_KEY is not configured")
	}
	input := make([]map[string]any, 0, len(repos))
	for _, repo := range repos {
		input = append(input, map[string]any{
			"full_name":   repo.FullName,
			"description": repo.Description,
			"language":    repo.Language,
			"topics":      repo.Topics,
			"archived":    repo.Archived,
			"disabled":    repo.Disabled,
			"fork":        repo.Fork,
			"stars":       repo.StargazersCount,
			"forks":       repo.ForksCount,
			"open_issues": repo.OpenIssuesCount,
			"pushed_at":   repo.PushedAt.UTC().Format(time.RFC3339),
			"starred_at":  starredAt(repo),
			"url":         repo.HTMLURL,
		})
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	prompt := `你是一个 GitHub Stars 整理助手。请根据仓库元数据判断用户是否还值得保留这个 Star。

评判原则：已归档、已禁用、长期没有更新、明显是一次性实验、重复/低价值 fork，优先建议取消；活跃、基础设施、学习资料、值得持续关注的项目应保留；信息不足时标记 review，不要武断。

	只返回一个 JSON 对象，不要 Markdown、不要解释。对象必须包含 reviews 数组，数组中的每一项必须包含：
{"reviews":[{"full_name":"owner/name","decision":"unstar|keep|review","score":0,"summary":"一句中文总结","reasons":["原因"],"next_action":"建议动作"}]}

score 是 0-100 的取消建议置信度。必须为输入中的每个仓库返回一项，并保持 full_name 不变。

仓库数据：` + string(data)
	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: "你只输出合法 JSON。"},
			{Role: "user", Content: prompt},
		},
		Temperature:    0.2,
		MaxTokens:      8000,
		ResponseFormat: aiReviewResponseFormat(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	if c.appURL != "" {
		req.Header.Set("HTTP-Referer", c.appURL)
		req.Header.Set("X-Title", "Review Stars")
	}
	started := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		logExternalError("openrouter", http.MethodPost, c.url, started, err)
		return nil, err
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	logExternalResponse("openrouter", http.MethodPost, c.url, resp.StatusCode, started, responseBody)
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openrouter API %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}
	var completion chatResponse
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		return nil, fmt.Errorf("decode openrouter response: %w", err)
	}
	if completion.Error != nil {
		return nil, errors.New(completion.Error.Message)
	}
	if len(completion.Choices) == 0 {
		return nil, errors.New("openrouter returned no choices")
	}
	return parseAIReviews(completion.Choices[0].Message.Content)
}

// appURL is kept on the client only for optional OpenRouter attribution headers.
func (c *OpenRouterClient) setAppURL(appURL string) { c.appURL = strings.TrimRight(appURL, "/") }

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
	return &chatResponseFormat{
		Type: "json_schema",
		JSONSchema: chatJSONSchema{
			Name:   "github_star_reviews",
			Strict: true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"reviews": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"full_name":   map[string]any{"type": "string"},
								"decision":    map[string]any{"type": "string", "enum": []string{"unstar", "keep", "review"}},
								"score":       map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
								"summary":     map[string]any{"type": "string"},
								"reasons":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
								"next_action": map[string]any{"type": "string"},
							},
							"required":             []string{"full_name", "decision", "score", "summary", "reasons", "next_action"},
							"additionalProperties": false,
						},
					},
				},
				"required":             []string{"reviews"},
				"additionalProperties": false,
			},
		},
	}
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
	Decision   string   `json:"decision"`
	Score      int      `json:"score"`
	Summary    string   `json:"summary"`
	Reasons    []string `json:"reasons"`
	NextAction string   `json:"next_action"`
	Source     string   `json:"source"`
}

func ruleReview(repo Repository, config Config, now time.Time) RepositoryReview {
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
				statusReasons = append(statusReasons, "仓库已归档")
			}
			if statuses["disabled"] && repo.Disabled {
				statusReasons = append(statusReasons, "仓库已被 GitHub 禁用")
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
			matchedReasons = append(matchedReasons, fmt.Sprintf("超过 %d 天未更新", config.RuleStaleDays))
		}
	}
	if config.RuleMaxStars > 0 {
		enabledRules++
		if repo.StargazersCount < config.RuleMaxStars {
			matchedRules++
			matchedReasons = append(matchedReasons, fmt.Sprintf("Star 少于 %d（当前 %d）", config.RuleMaxStars, repo.StargazersCount))
		}
	}
	decision := "keep"
	score := 0
	reasons := []string(nil)
	summary := "未同时命中配置规则"
	if enabledRules == 0 {
		summary = "没有启用规则"
	} else if matchedRules == enabledRules {
		decision = "unstar"
		score = 100
		reasons = matchedReasons
		summary = fmt.Sprintf("同时命中 %d 条规则", enabledRules)
	}
	return RepositoryReview{
		Repository: repo,
		Decision:   decision,
		Score:      score,
		Summary:    summary,
		Reasons:    reasons,
		NextAction: nextActionFor(decision),
		Source:     "rule",
	}
}

func nextActionFor(decision string) string {
	switch decision {
	case "unstar":
		return "确认后取消 Star"
	case "review":
		return "打开仓库快速回顾"
	default:
		return "继续保留"
	}
}

func mergeAIReviews(repos []Repository, aiReviews []AIReview) ([]RepositoryReview, error) {
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
			Repository: repo,
			Decision:   review.Decision,
			Score:      review.Score,
			Summary:    review.Summary,
			Reasons:    review.Reasons,
			NextAction: review.NextAction,
			Source:     "openrouter",
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

func preferredReviews(aiReviews, ruleReviews []RepositoryReview) []RepositoryReview {
	if len(aiReviews) > 0 {
		return aiReviews
	}
	return ruleReviews
}

type Store struct {
	mu          sync.RWMutex
	repos       []Repository
	reviews     []RepositoryReview
	ruleReviews []RepositoryReview
	updatedAt   time.Time
}

func (s *Store) setData(repos []Repository, reviews []RepositoryReview) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repos = append([]Repository{}, repos...)
	s.reviews = append([]RepositoryReview{}, reviews...)
	s.updatedAt = time.Now()
}

func (s *Store) setRuleReviews(reviews []RepositoryReview) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ruleReviews = append([]RepositoryReview{}, reviews...)
	s.updatedAt = time.Now()
}

func (s *Store) snapshot() ([]Repository, []RepositoryReview, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Repository{}, s.repos...), append([]RepositoryReview{}, s.reviews...), s.updatedAt
}

func (s *Store) ruleSnapshot() ([]RepositoryReview, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]RepositoryReview{}, s.ruleReviews...), s.updatedAt
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
	filteredRuleReviews := s.ruleReviews[:0]
	for _, review := range s.ruleReviews {
		if !strings.EqualFold(review.FullName, fullName) {
			filteredRuleReviews = append(filteredRuleReviews, review)
		}
	}
	s.ruleReviews = filteredRuleReviews
}

func cacheKey(fullName string) string {
	return strings.ToLower(strings.TrimSpace(fullName))
}

func repositoryFingerprint(repo Repository) string {
	snapshot := struct {
		FullName    string     `json:"full_name"`
		Description string     `json:"description"`
		Language    string     `json:"language"`
		Topics      []string   `json:"topics"`
		Archived    bool       `json:"archived"`
		Disabled    bool       `json:"disabled"`
		Fork        bool       `json:"fork"`
		Stars       int        `json:"stars"`
		Forks       int        `json:"forks"`
		OpenIssues  int        `json:"open_issues"`
		PushedAt    time.Time  `json:"pushed_at"`
		UpdatedAt   time.Time  `json:"updated_at"`
		StarredAt   *time.Time `json:"starred_at,omitempty"`
	}{
		FullName: repo.FullName, Description: repo.Description, Language: repo.Language,
		Topics: repo.Topics, Archived: repo.Archived, Disabled: repo.Disabled, Fork: repo.Fork,
		Stars: repo.StargazersCount, Forks: repo.ForksCount, OpenIssues: repo.OpenIssuesCount,
		PushedAt: repo.PushedAt, UpdatedAt: repo.UpdatedAt, StarredAt: repo.StarredAt,
	}
	data, _ := json.Marshal(snapshot)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type App struct {
	config   Config
	github   *GitHubClient
	ai       *OpenRouterClient
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

func (a *App) syncAllStars(ctx context.Context) ([]Repository, []RepositoryReview, []RepositoryReview, error) {
	repos, err := a.github.ListStarred(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	if a.db == nil {
		return nil, nil, nil, errors.New("SQLite database is not configured")
	}
	if err := a.db.SaveRepositories(repos); err != nil {
		return nil, nil, nil, fmt.Errorf("save repositories to sqlite: %w", err)
	}
	reviews, err := a.db.ListReviews(repos)
	if err != nil {
		return nil, nil, nil, err
	}
	ruleReviews, err := a.db.ListRuleReviews(repos)
	if err != nil {
		return nil, nil, nil, err
	}
	a.store.setData(repos, reviews)
	a.store.setRuleReviews(ruleReviews)
	return repos, reviews, ruleReviews, nil
}

func (a *App) reviewAll(ctx context.Context, force bool) ([]RepositoryReview, ReviewStats, int, int, int, []string, error) {
	a.reviewMu.Lock()
	defer a.reviewMu.Unlock()

	repos, _, _ := a.store.snapshot()
	if len(repos) == 0 {
		return nil, ReviewStats{}, 0, 0, 0, nil, errors.New("还没有同步仓库，请先点击“同步仓库”")
	}
	if a.db == nil {
		return nil, ReviewStats{}, 0, 0, 0, nil, errors.New("SQLite database is not configured")
	}
	byName := make(map[string]RepositoryReview, len(repos))
	uncached := make([]Repository, 0, len(repos))
	cachedCount := 0
	for _, repo := range repos {
		if !force {
			cached, ok, err := a.db.GetReview(repo)
			if err != nil {
				return nil, ReviewStats{}, 0, 0, 0, nil, err
			}
			if ok && cached.Source == "openrouter" {
				byName[cacheKey(repo.FullName)] = cached
				cachedCount++
				continue
			}
		}
		uncached = append(uncached, repo)
	}

	batchSize := a.config.ReviewBatchSize
	if batchSize < 1 {
		batchSize = 20
	}
	batchCount := 0
	aiReviewedCount := 0
	warnings := make([]string, 0)
	for start := 0; start < len(uncached); start += batchSize {
		end := start + batchSize
		if end > len(uncached) {
			end = len(uncached)
		}
		batch := uncached[start:end]
		batchCount++
		if a.ai == nil {
			return nil, ReviewStats{}, cachedCount, aiReviewedCount, batchCount, warnings, errors.New("OPENROUTER_API_KEY is not configured; run rule review instead")
		}
		aiReviews, err := a.ai.Review(ctx, batch)
		if err != nil {
			return nil, ReviewStats{}, cachedCount, aiReviewedCount, batchCount, warnings, fmt.Errorf("AI review failed in batch %d: %w", batchCount, err)
		}
		merged, err := mergeAIReviews(batch, aiReviews)
		if err != nil {
			return nil, ReviewStats{}, cachedCount, aiReviewedCount, batchCount, warnings, fmt.Errorf("AI review result invalid in batch %d: %w", batchCount, err)
		}
		for _, review := range merged {
			byName[cacheKey(review.FullName)] = review
			if err := a.db.SaveReview(review); err != nil {
				return nil, ReviewStats{}, cachedCount, aiReviewedCount, batchCount, warnings, fmt.Errorf("save AI review to sqlite: %w", err)
			}
			aiReviewedCount++
		}
	}

	reviews := make([]RepositoryReview, 0, len(repos))
	for _, repo := range repos {
		if review, ok := byName[cacheKey(repo.FullName)]; ok {
			reviews = append(reviews, review)
		} else {
			return nil, ReviewStats{}, cachedCount, aiReviewedCount, batchCount, warnings, fmt.Errorf("AI review did not produce a result for %s", repo.FullName)
		}
	}
	a.store.setData(repos, reviews)
	return reviews, statsFor(reviews), cachedCount, aiReviewedCount, batchCount, warnings, nil
}

func (a *App) sync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	repos, reviews, ruleReviews, err := a.syncAllStars(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repositories": repos,
		"reviews":      reviews,
		"rule_reviews": ruleReviews,
		"count":        len(repos),
		"stats":        statsForCollection(repos, reviews),
		"rule_stats":   statsForCollection(repos, ruleReviews),
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
		writeJSON(w, http.StatusOK, map[string]any{"reviews": reviews, "stats": statsForCollection(repos, reviews), "updated_at": updatedAt})
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
	}
	reviews, stats, cachedCount, aiReviewedCount, batchCount, warnings, err := a.reviewAll(r.Context(), force)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	response := map[string]any{
		"reviews":           reviews,
		"stats":             stats,
		"cached_count":      cachedCount,
		"ai_reviewed_count": aiReviewedCount,
		"batch_count":       batchCount,
		"updated_at":        time.Now(),
	}
	if len(warnings) > 0 {
		response["warning"] = strings.Join(warnings, "\n")
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) ruleReviewAll() ([]RepositoryReview, ReviewStats, error) {
	a.reviewMu.Lock()
	defer a.reviewMu.Unlock()

	repos, _, _ := a.store.snapshot()
	if len(repos) == 0 {
		return nil, ReviewStats{}, errors.New("还没有同步仓库，请先点击“同步仓库”")
	}
	if a.db == nil {
		return nil, ReviewStats{}, errors.New("SQLite database is not configured")
	}
	reviews := make([]RepositoryReview, 0, len(repos))
	for _, repo := range repos {
		review := ruleReview(repo, a.config, time.Now())
		if err := a.db.SaveRuleReview(review); err != nil {
			return nil, ReviewStats{}, fmt.Errorf("save rule review to sqlite: %w", err)
		}
		reviews = append(reviews, review)
	}
	a.store.setRuleReviews(reviews)
	return reviews, statsFor(reviews), nil
}

func (a *App) ruleReviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		repos, _, _ := a.store.snapshot()
		reviews, updatedAt := a.store.ruleSnapshot()
		if len(reviews) == 0 {
			writeError(w, http.StatusNotFound, errors.New("还没有规则评审结果，请先运行规则评审"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"reviews": reviews, "stats": statsForCollection(repos, reviews), "updated_at": updatedAt})
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	reviews, stats, err := a.ruleReviewAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reviews": reviews,
		"stats":   stats,
		"rule_config": map[string]any{
			"statuses":   a.config.RuleStatuses,
			"stale_days": a.config.RuleStaleDays,
			"max_stars":  a.config.RuleMaxStars,
		},
		"updated_at": time.Now(),
	})
}

func (a *App) randomRepository(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	_, aiReviews, _ := a.store.snapshot()
	ruleReviews, _ := a.store.ruleSnapshot()
	reviews := preferredReviews(aiReviews, ruleReviews)
	if len(reviews) > 0 {
		repo := reviews[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(reviews))]
		writeJSON(w, http.StatusOK, map[string]any{"repository": repo})
		return
	}
	writeError(w, http.StatusNotFound, errors.New("还没有 AI 或规则评审结果，请先运行一种评审"))
}

func (a *App) unstar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	fullName := strings.TrimPrefix(r.URL.Path, "/api/stars/")
	fullName, _ = url.PathUnescape(fullName)
	if err := a.github.Unstar(r.Context(), fullName); err != nil {
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

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                    true,
		"github_configured":     a.config.GitHubToken != "",
		"openrouter_configured": a.config.OpenRouterAPIKey != "",
		"telegram_configured":   a.config.TelegramBotToken != "" && a.config.TelegramChatID != "",
		"model":                 a.config.OpenRouterModel,
		"database_file":         a.config.DatabaseFile,
		"review_cron":           a.config.ReviewCron,
		"review_count":          a.config.ReviewCount,
		"rule_statuses":         a.config.RuleStatuses,
		"rule_stale_days":       a.config.RuleStaleDays,
		"rule_max_stars":        a.config.RuleMaxStars,
	})
}

func (a *App) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", a.health)
	mux.HandleFunc("/api/stars", a.listStars)
	mux.HandleFunc("/api/stars/", a.unstar)
	mux.HandleFunc("/api/sync", a.sync)
	mux.HandleFunc("/api/review", a.review)
	mux.HandleFunc("/api/rule-review", a.ruleReviewHandler)
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
	ai := &OpenRouterClient{url: config.OpenRouterURL, key: config.OpenRouterAPIKey, model: config.OpenRouterModel, http: httpClient}
	ai.setAppURL(config.AppURL)
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
	ruleReviews, err := database.ListRuleReviews(repos)
	if err != nil {
		log.Fatalf("load rule reviews from sqlite: %v", err)
	}
	store := &Store{}
	store.setData(repos, reviews)
	store.setRuleReviews(ruleReviews)
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
	log.Printf("Review Stars listening on http://%s:%s (loaded repositories=%d ai_reviews=%d rule_reviews=%d database=%s)", config.Host, config.Port, len(repos), len(reviews), len(ruleReviews), config.DatabaseFile)
	if config.GitHubToken == "" {
		log.Printf("warning: GITHUB_TOKEN is not configured")
	}
	if config.OpenRouterAPIKey == "" {
		log.Printf("warning: OPENROUTER_API_KEY is not configured; AI review is unavailable, rule review remains available")
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
