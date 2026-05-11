package main

import "time"

// FeedConfig holds a feed URL and an optional post-processor.
type FeedConfig struct {
	URL         string
	Postprocess func([]RSSItem) []RSSItem
}

type RSSItem struct {
	FeedTitle string
	Title     string
	Link      string
	Content   string
	Published time.Time
}
