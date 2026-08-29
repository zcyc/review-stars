package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseAIReviews(t *testing.T) {
	content := "```json\n[{\"full_name\":\"acme/demo\",\"decision\":\"UNSTAR\",\"score\":120,\"summary\":\"已归档\"}]\n```"
	reviews, err := parseAIReviews(content)
	if err != nil {
		t.Fatalf("parseAIReviews() error = %v", err)
	}
	if len(reviews) != 1 || reviews[0].Decision != "unstar" || reviews[0].Score != 100 {
		t.Fatalf("unexpected review: %#v", reviews)
	}
}

func TestDecodeStarredRepositoriesEnvelope(t *testing.T) {
	data := []byte(`[{"starred_at":"2026-03-12T00:00:00Z","repo":{"id":42,"name":"demo","full_name":"acme/demo","html_url":"https://github.com/acme/demo","description":"Demo","stargazers_count":1234,"archived":true,"pushed_at":"2026-03-10T00:00:00Z","updated_at":"2026-03-11T00:00:00Z"}}]`)
	repos, err := decodeStarredRepositories(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("got %d repositories", len(repos))
	}
	if repos[0].FullName != "acme/demo" || repos[0].StargazersCount != 1234 || !repos[0].Archived {
		t.Fatalf("repository fields were not decoded: %#v", repos[0])
	}
	if repos[0].StarredAt == nil || repos[0].StarredAt.Year() != 2026 {
		t.Fatalf("starred_at was not decoded: %#v", repos[0].StarredAt)
	}
}

func TestDecodeStarredRepositoriesRootShape(t *testing.T) {
	data := []byte(`[{"name":"demo","full_name":"acme/demo","stargazers_count":9,"archived":false,"starred_at":"2026-03-12T00:00:00Z"}]`)
	repos, err := decodeStarredRepositories(data)
	if err != nil || len(repos) != 1 || repos[0].Name != "demo" || repos[0].StargazersCount != 9 {
		t.Fatalf("root repository shape was not decoded: %#v, %v", repos, err)
	}
}

func TestParseAIReviewsEnvelopeAndPrefix(t *testing.T) {
	content := "Here is the result:\n{\"reviews\":[{\"full_name\":\"acme/demo\",\"decision\":\"keep\",\"score\":4,\"summary\":\"仍然活跃\",\"reasons\":[\"最近有更新\"]}]}"
	reviews, err := parseAIReviews(content)
	if err != nil {
		t.Fatalf("parseAIReviews() error = %v", err)
	}
	if len(reviews) != 1 || reviews[0].FullName != "acme/demo" || reviews[0].Decision != "keep" {
		t.Fatalf("unexpected envelope review: %#v", reviews)
	}
}

func TestParseAIReviewsIncludesResponsePreviewOnFailure(t *testing.T) {
	_, err := parseAIReviews("Unable to produce JSON")
	if err == nil || !strings.Contains(err.Error(), "Unable to produce JSON") {
		t.Fatalf("expected response preview in error, got %v", err)
	}
}

func TestRuleReview(t *testing.T) {
	repo := Repository{FullName: "acme/old", Archived: true, StargazersCount: 12, PushedAt: time.Now().Add(-2 * 365 * 24 * time.Hour)}
	review := ruleReview(repo, Config{RuleStatuses: "archived", RuleStaleDays: 180, RuleMaxStars: 1000}, time.Now())
	if review.Decision != "unstar" || review.Score != 100 {
		t.Fatalf("expected rule-matched repo to be unstar, got %#v", review)
	}
	if len(review.Reasons) == 0 {
		t.Fatal("expected at least one reason")
	}
	partial := ruleReview(Repository{FullName: "acme/partial", Archived: true, StargazersCount: 5000, PushedAt: time.Now()}, Config{RuleStatuses: "archived", RuleStaleDays: 180, RuleMaxStars: 1000}, time.Now())
	if partial.Decision != "keep" || len(partial.Reasons) != 0 {
		t.Fatalf("partially matched rules should not trigger: %#v", partial)
	}
}

func TestApplyRulePrefilter(t *testing.T) {
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	ruleRepo := Repository{FullName: "acme/old", Archived: true, StargazersCount: 12, PushedAt: now.Add(-365 * 24 * time.Hour)}
	aiRepo := Repository{FullName: "acme/active", PushedAt: now.Add(-24 * time.Hour)}
	aiReview := RepositoryReview{Repository: aiRepo, Source: "ai", AILanguage: "zh-CN", Decision: "keep"}
	result := applyRulePrefilter([]Repository{ruleRepo, aiRepo}, []RepositoryReview{aiReview}, Config{Language: "zh-CN", RuleStatuses: "archived", RuleStaleDays: 180, RuleMaxStars: 1000}, now)
	if len(result) != 2 || result[0].Source != "rule" || result[0].Decision != "unstar" || result[1].Source != "ai" {
		t.Fatalf("unexpected prefiltered reviews: %#v", result)
	}
}

func TestMergeReviewsKeepsRepositoryData(t *testing.T) {
	repo := Repository{FullName: "acme/demo", Name: "demo", HTMLURL: "https://github.com/acme/demo"}
	merged, err := mergeAIReviews([]Repository{repo}, []AIReview{{FullName: "acme/demo", Decision: "keep", Score: 8, Summary: "仍然活跃", Reasons: []string{"有持续更新"}}}, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 || merged[0].HTMLURL != repo.HTMLURL {
		t.Fatalf("repository metadata was lost: %#v", merged)
	}
	if merged[0].Decision != "keep" || merged[0].Source != "ai" {
		t.Fatalf("AI review was not merged: %#v", merged[0])
	}
	if merged[0].AILanguage != "zh-CN" {
		t.Fatalf("AI review language = %q, want zh-CN", merged[0].AILanguage)
	}
	if merged[0].ReviewLanguage != "zh-CN" {
		t.Fatalf("review language = %q, want zh-CN", merged[0].ReviewLanguage)
	}
}

func TestAIClientUsesConfiguredOpenAIEndpoint(t *testing.T) {
	client := &AIClient{baseURL: "https://api.example.test/v1", key: "test-key", model: "deepseek-v4-flash", thinking: "disabled", maxTokens: 16000, http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q, want Bearer test-key", got)
		}
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Model != "deepseek-v4-flash" {
			t.Errorf("model = %q, want deepseek-v4-flash", request.Model)
		}
		if request.MaxTokens != 16000 {
			t.Errorf("max tokens = %d, want 16000", request.MaxTokens)
		}
		if request.Thinking == nil || request.Thinking.Type != "disabled" {
			t.Errorf("thinking = %#v, want disabled", request.Thinking)
		}
		if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" {
			t.Errorf("response format = %#v, want json_object", request.ResponseFormat)
		}
		return &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"{\"reviews\":[{\"full_name\":\"acme/demo\",\"decision\":\"keep\",\"score\":1,\"summary\":\"活跃\",\"reasons\":[\"有更新\"]}]}"}}],"usage":{"prompt_tokens":120,"completion_tokens":40,"total_tokens":160,"prompt_cache_hit_tokens":20}}`)),
		}, nil
	})}}
	reviews, usage, err := client.Review(context.Background(), []Repository{{FullName: "acme/demo"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].FullName != "acme/demo" {
		t.Fatalf("unexpected AI reviews: %#v", reviews)
	}
	if usage.PromptTokens != 120 || usage.CompletionTokens != 40 || usage.PromptCacheHitTokens != 20 || usage.TotalTokens != 160 {
		t.Fatalf("unexpected AI usage: %#v", usage)
	}
}

func TestAIClientOmitsDeepSeekThinkingForGenericModel(t *testing.T) {
	client := &AIClient{
		baseURL:  "https://api.example.test/v1",
		key:      "test-key",
		model:    "gpt-5.4",
		thinking: "disabled",
		http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var request chatRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if request.Thinking != nil {
				t.Errorf("thinking = %#v, want omitted for generic model", request.Thinking)
			}
			return &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"{\"reviews\":[{\"full_name\":\"acme/demo\",\"decision\":\"keep\"}]}"}}]}`)),
			}, nil
		})},
	}
	if _, _, err := client.Review(context.Background(), []Repository{{FullName: "acme/demo"}}); err != nil {
		t.Fatal(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestEnvAIThinkingDefaultsToDisabled(t *testing.T) {
	t.Setenv("AI_THINKING", "")
	if got := envAIThinking(); got != "disabled" {
		t.Fatalf("default AI thinking = %q, want disabled", got)
	}
	t.Setenv("AI_THINKING", "enabled")
	if got := envAIThinking(); got != "enabled" {
		t.Fatalf("enabled AI thinking = %q, want enabled", got)
	}
	t.Setenv("AI_THINKING", "unexpected")
	if got := envAIThinking(); got != "disabled" {
		t.Fatalf("invalid AI thinking = %q, want disabled", got)
	}
}

func TestEnvLanguageDefaultsToChinese(t *testing.T) {
	t.Setenv("APP_LANGUAGE", "")
	if got := envLanguage(); got != "zh-CN" {
		t.Fatalf("default language = %q, want zh-CN", got)
	}
	t.Setenv("APP_LANGUAGE", "en-US")
	if got := envLanguage(); got != "en" {
		t.Fatalf("English language = %q, want en", got)
	}
	t.Setenv("APP_LANGUAGE", "fr")
	if got := envLanguage(); got != "zh-CN" {
		t.Fatalf("unsupported language = %q, want zh-CN", got)
	}
}

func TestAIReviewCompleteRequiresAllRepositoriesInConfiguredLanguage(t *testing.T) {
	repos := []Repository{{FullName: "acme/one"}, {FullName: "acme/two"}}
	complete := []RepositoryReview{
		{Repository: repos[0], Source: "ai", AILanguage: "zh-CN"},
		{Repository: repos[1], Source: "ai", AILanguage: "zh-CN"},
	}
	if !aiReviewComplete(repos, complete, "zh-CN") {
		t.Fatal("complete AI reviews were not recognized")
	}
	if aiReviewComplete(repos, complete[:1], "zh-CN") {
		t.Fatal("incomplete AI reviews were recognized as complete")
	}
	withRule := []RepositoryReview{{Repository: repos[0], Source: "rule", ReviewLanguage: "zh-CN"}, complete[1]}
	if !aiReviewComplete(repos, withRule, "zh-CN") {
		t.Fatal("a rule result and an AI result were not recognized as complete")
	}
	complete[1].AILanguage = "en"
	if aiReviewComplete(repos, complete, "zh-CN") {
		t.Fatal("reviews in another language were recognized as complete")
	}
}

func TestUnstarArchivedRepositoryUsesGitHubAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/user/starred/acme/old" {
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	store := &Store{}
	store.setData([]Repository{{FullName: "acme/old", Archived: true}}, nil)
	app := &App{github: &GitHubClient{baseURL: server.URL, token: "test", http: server.Client()}, store: store}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/stars/acme/old", nil)

	app.unstar(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("archived unstar status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestReviewGetReturnsPendingStats(t *testing.T) {
	store := &Store{}
	store.setData([]Repository{{FullName: "acme/one"}, {FullName: "acme/two"}}, nil)
	app := &App{store: store}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/review", nil)

	app.review(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("review GET status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response struct {
		Reviews []RepositoryReview `json:"reviews"`
		Stats   ReviewStats        `json:"stats"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Reviews) != 0 || response.Stats != (ReviewStats{Total: 2, Review: 2}) {
		t.Fatalf("unexpected pending review response: %#v", response)
	}
}

func TestUnstarPermissionErrorReturnsManualStarsLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/user/starred/acme/demo" {
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("Accept = %q, want application/vnd.github+json", got)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by personal access token","status":"403"}`))
	}))
	defer server.Close()

	store := &Store{}
	store.setData([]Repository{{FullName: "acme/demo"}}, nil)
	app := &App{github: &GitHubClient{baseURL: server.URL, token: "test", http: server.Client()}, store: store}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/stars/acme/demo", nil)

	app.unstar(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("permission error status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	var response struct {
		Error    string `json:"error"`
		StarsURL string `json:"stars_url"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Error, "Starring") || response.StarsURL != "https://github.com/stars?q=acme%2Fdemo" {
		t.Fatalf("unexpected permission response: %#v", response)
	}
}

func TestStatsFor(t *testing.T) {
	reviews := []RepositoryReview{{Decision: "unstar"}, {Decision: "review"}, {Decision: "keep"}, {Decision: "keep"}}
	got := statsFor(reviews)
	want := ReviewStats{Total: 4, Unstar: 1, Review: 1, Keep: 2}
	if got != want {
		t.Fatalf("statsFor() = %#v, want %#v", got, want)
	}
}

func TestStatsForCollectionCountsPendingRepositories(t *testing.T) {
	repos := []Repository{{FullName: "acme/one"}, {FullName: "acme/two"}, {FullName: "acme/three"}}
	reviews := []RepositoryReview{{Repository: repos[0], Decision: "keep"}}
	got := statsForCollection(repos, reviews)
	want := ReviewStats{Total: 3, Review: 2, Keep: 1}
	if got != want {
		t.Fatalf("statsForCollection() = %#v, want %#v", got, want)
	}
}

func TestRepositoryFingerprintIgnoresVolatileMetadata(t *testing.T) {
	base := Repository{
		FullName:        "acme/demo",
		Description:     "A repository",
		Language:        "Go",
		Topics:          []string{"github", "tools"},
		StargazersCount: 10,
		ForksCount:      2,
		OpenIssuesCount: 1,
		PushedAt:        time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		StarredAt:       timePtr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	}
	volatile := base
	volatile.StargazersCount = 9999
	volatile.ForksCount = 999
	volatile.OpenIssuesCount = 99
	volatile.UpdatedAt = time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	volatile.StarredAt = timePtr(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if repositoryFingerprint(base) != repositoryFingerprint(volatile) {
		t.Fatal("volatile repository metadata changed the fingerprint")
	}

	nextMonth := base
	nextMonth.PushedAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if repositoryFingerprint(base) == repositoryFingerprint(nextMonth) {
		t.Fatal("a new pushed month did not change the fingerprint")
	}

	changedDescription := base
	changedDescription.Description = "A different repository"
	if repositoryFingerprint(base) == repositoryFingerprint(changedDescription) {
		t.Fatal("a changed description did not change the fingerprint")
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func TestServeSPA(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost/", nil)
	serveSPA(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("serveSPA() status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("serveSPA() content type = %q", recorder.Header().Get("Content-Type"))
	}
}

func TestLoadDotEnvDoesNotOverwriteExistingEnv(t *testing.T) {
	const key = "REVIEW_STARS_DOTENV_TEST"
	t.Setenv(key, "from-shell")
	filename := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(filename, []byte(key+"=from-file\nOTHER_REVIEW_STARS_KEY=loaded\n"), 0600); err != nil {
		t.Fatal(err)
	}
	loadDotEnv(filename)
	if got := os.Getenv(key); got != "from-shell" {
		t.Fatalf("loadDotEnv() overwrote shell value with %q", got)
	}
	if got := os.Getenv("OTHER_REVIEW_STARS_KEY"); got != "loaded" {
		t.Fatalf("loadDotEnv() did not load file value: %q", got)
	}
	t.Cleanup(func() { _ = os.Unsetenv("OTHER_REVIEW_STARS_KEY") })
}

func TestSQLiteStoreRoundTripAndFingerprint(t *testing.T) {
	repo := Repository{
		FullName:    "acme/demo",
		Description: "demo project",
		PushedAt:    time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
	}
	database, err := openSQLiteStore(filepath.Join(t.TempDir(), "review-stars.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.SaveRepositories([]Repository{repo}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveReview(RepositoryReview{
		Repository: repo,
		Decision:   "keep",
		Score:      5,
		Summary:    "仍然活跃",
		Source:     "ai",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveRuleReview(ruleReview(repo, Config{RuleStatuses: "archived", RuleStaleDays: 180, RuleMaxStars: 1000}, time.Now())); err != nil {
		t.Fatal(err)
	}
	review, ok, err := database.GetReview(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || review.Decision != "keep" {
		t.Fatalf("expected stored review, got %#v, %v", review, ok)
	}
	changed := repo
	changed.Description = "changed metadata"
	if _, ok, err := database.GetReview(changed); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("changed repository metadata should invalidate the stored review")
	}
	repos, err := database.ListRepositories()
	if err != nil || len(repos) != 1 {
		t.Fatalf("unexpected stored repositories: %#v, %v", repos, err)
	}
	reviews, err := database.ListReviews(repos)
	if err != nil || len(reviews) != 1 {
		t.Fatalf("unexpected stored reviews: %#v, %v", reviews, err)
	}
	ruleReviews, err := database.ListRuleReviews(repos)
	if err != nil || len(ruleReviews) != 1 || ruleReviews[0].Source != "rule" {
		t.Fatalf("unexpected stored rule reviews: %#v, %v", ruleReviews, err)
	}
	if err := database.DeleteRepository("ACME/DEMO"); err != nil {
		t.Fatal(err)
	}
	if repos, err := database.ListRepositories(); err != nil || len(repos) != 0 {
		t.Fatalf("case-insensitive repository delete failed: %#v, %v", repos, err)
	}
}

func TestUnstarDoesNotReportSuccessWhenLocalCleanupFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	database, err := openSQLiteStore(filepath.Join(t.TempDir(), "review-stars.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveRepositories([]Repository{{FullName: "acme/demo"}}); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	store := &Store{}
	store.setData([]Repository{{FullName: "acme/demo"}}, nil)
	app := &App{
		github: &GitHubClient{baseURL: server.URL, token: "test", http: server.Client()},
		store:  store,
		db:     database,
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/stars/acme/demo", nil)

	app.unstar(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("cleanup failure status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	repos, _, _ := store.snapshot()
	if len(repos) != 1 {
		t.Fatalf("repository was removed from memory after cleanup failure: %#v", repos)
	}
}

func TestRedactExternalURL(t *testing.T) {
	got := redactExternalURL("https://api.telegram.org/bot123456:secret/sendMessage")
	if strings.Contains(got, "secret") || !strings.Contains(got, "/bot-REDACTED/sendMessage") {
		t.Fatalf("Telegram token was not redacted: %s", got)
	}
	githubURL := "https://api.github.com/user/starred"
	if redactExternalURL(githubURL) != githubURL {
		t.Fatalf("non-Telegram URL changed: %s", redactExternalURL(githubURL))
	}
}

func TestLogExternalResponsePrintsCompleteBody(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	body := "start\n" + strings.Repeat("x", 3000) + "\nend"
	logExternalResponse("ai", http.MethodPost, "https://api.example.test/chat/completions", http.StatusOK, time.Now(), []byte(body))
	if !strings.Contains(output.String(), body) {
		t.Fatalf("response log did not include complete body")
	}
}
