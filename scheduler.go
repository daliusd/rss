package main

import (
	"fmt"
	"log"
	"time"
)

// ====================================================================
// Scheduler
// ====================================================================

func runDailyJob(store *Store, seen *SeenStore) {
	seen.Cleanup()
	log.Println("[rss] fetching feeds…")
	items := fetchFeeds()
	now := time.Now().UTC()
	store.Set(items, now)
	log.Printf("[rss] fetched %d items at %s", len(items), now.Format(time.RFC3339))

	// Filter to items not previously emailed and no older than 7 days.
	cutoff := now.AddDate(0, 0, -7)
	var newItems []RSSItem
	for _, item := range items {
		if seen.IsNew(item.Link) && !item.Published.Before(cutoff) {
			newItems = append(newItems, item)
		}
	}
	log.Printf("[email] %d new item(s) to send", len(newItems))

	if len(newItems) == 0 {
		return
	}

	htmlBody, err := buildEmailHTML(newItems, now)
	if err != nil {
		log.Printf("[email] build error: %v", err)
		return
	}

	subject := fmt.Sprintf("RSS Digest — %d new item(s) on %s",
		len(newItems), now.Format("2 Jan 2006"))

	if err := sendEmail(subject, htmlBody); err != nil {
		log.Printf("[email] send error: %v", err)
		return
	}
	log.Printf("[email] sent digest with %d item(s) to %s", len(newItems), mailTo)

	// Persist only after a successful send.
	links := make([]string, len(newItems))
	for i, item := range newItems {
		links[i] = item.Link
	}
	seen.MarkSeen(links)
}

// nextRunAt returns the next occurrence of hh:mm UTC on or after now.
func nextRunAt(now time.Time, hour, min int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func scheduler(store *Store, seen *SeenStore) {
	runDailyJob(store, seen)

	for {
		next := nextRunAt(time.Now().UTC(), 5, 0)
		log.Printf("[scheduler] next run at %s", next.Format(time.RFC3339))
		<-time.After(time.Until(next))
		runDailyJob(store, seen)
	}
}
