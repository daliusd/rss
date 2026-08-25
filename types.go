package main

import "time"

// FeedConfig holds a feed URL.
type FeedConfig struct {
	URL string
}

type RSSItem struct {
	FeedTitle string
	Title     string
	Link      string
	Content   string
	Published time.Time
}
