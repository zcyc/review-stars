package main

import (
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
	content := "Here is the result:\n{\"reviews\":[{\"full_name\":\"acme/demo\",\"decision\":\"keep\",\"score\":4,\"summary\":\"仍然活跃\",\"reasons\":[\"最近有更新\"],\"next_action\":\"继续保留\"}]}"
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

func TestMergeReviewsKeepsRepositoryData(t *testing.T) {
	repo := Repository{FullName: "acme/demo", Name: "demo", HTMLURL: "https://github.com/acme/demo"}
	merged, err := mergeAIReviews([]Repository{repo}, []AIReview{{FullName: "acme/demo", Decision: "keep", Score: 8, Summary: "仍然活跃", Reasons: []string{"有持续更新"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 || merged[0].HTMLURL != repo.HTMLURL {
		t.Fatalf("repository metadata was lost: %#v", merged)
	}
	if merged[0].Decision != "keep" || merged[0].Source != "openrouter" {
		t.Fatalf("AI review was not merged: %#v", merged[0])
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
		Source:     "openrouter",
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
