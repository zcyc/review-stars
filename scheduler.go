package main

import (
	"context"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

func startReminderScheduler(app *App) *cron.Cron {
	if strings.TrimSpace(app.config.ReviewCron) == "" {
		return nil
	}
	if app.telegram == nil || app.config.TelegramBotToken == "" || app.config.TelegramChatID == "" {
		log.Printf("warning: REVIEW_CRON is configured but Telegram is not configured; scheduler disabled")
		return nil
	}
	scheduler := cron.New()
	if _, err := scheduler.AddFunc(app.config.ReviewCron, func() {
		app.sendScheduledReminders()
	}); err != nil {
		log.Printf("warning: invalid REVIEW_CRON %q: %v; scheduler disabled", app.config.ReviewCron, err)
		return nil
	}
	scheduler.Start()
	log.Printf("telegram reminder scheduler started cron=%q count=%d", app.config.ReviewCron, app.config.ReviewCount)
	return scheduler
}

func (a *App) sendScheduledReminders() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, aiReviews, _ := a.store.snapshot()
	ruleReviews, _ := a.store.ruleSnapshot()
	reviews := preferredReviews(aiReviews, ruleReviews)
	if len(reviews) == 0 {
		log.Printf("[telegram-scheduler] skipped: no repositories in sqlite; sync first")
		return
	}
	selected := randomReviewBatch(reviews, a.config.ReviewCount)
	for _, review := range selected {
		if err := a.telegram.Send(ctx, review); err != nil {
			log.Printf("[telegram-scheduler] failed full_name=%s error=%v", review.FullName, err)
			continue
		}
		log.Printf("[telegram-scheduler] sent full_name=%s", review.FullName)
	}
}

func randomReviewBatch(reviews []RepositoryReview, count int) []RepositoryReview {
	if count < 1 {
		count = 1
	}
	if count > len(reviews) {
		count = len(reviews)
	}
	permutation := rand.New(rand.NewSource(time.Now().UnixNano())).Perm(len(reviews))
	selected := make([]RepositoryReview, 0, count)
	for _, index := range permutation[:count] {
		selected = append(selected, reviews[index])
	}
	return selected
}
