package main

import (
	"log"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"
)

// ====================================================================
// Feed configuration
// ====================================================================

var feedConfigs = []FeedConfig{
	{URL: "https://addyosmani.com/rss.xml"},
	{URL: "https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_anthropic_engineering.xml"},
	{URL: "https://www.reddit.com/r/neovim/top/.rss?t=week"},
	{URL: "https://www.reddit.com/r/lithuania/top/.rss?t=week"},
	{URL: "https://www.reddit.com/r/israel/top/.rss?t=week"},
	{URL: "https://www.reddit.com/r/ukraine/top/.rss?t=week"},
	{URL: "https://www.youtube.com/feeds/videos.xml?channel_id=UCswG6FSbgZjbWtdf_hMLaow"},
	{URL: "https://www.youtube.com/feeds/videos.xml?channel_id=UCLKPca3kwwd-B59HNr-_lvA"},
}

// ====================================================================
// RSS fetch logic
// ====================================================================

const feedFetchPause = 15 * time.Second

func fetchFeeds() []RSSItem {
	return fetchFeedsWith(fetchFeed, feedFetchPause)
}

func fetchFeed(url string) (*gofeed.Feed, error) {
	parser := gofeed.NewParser()
	parser.UserAgent = "rss-reader/1.0"
	parser.Client = &http.Client{Timeout: 30 * time.Second}
	return parser.ParseURL(url)
}

// fetchFeedsWith is the sequential fetch implementation, with its network
// operation and pause injectable for testing.
func fetchFeedsWith(fetch func(string) (*gofeed.Feed, error), pause time.Duration) []RSSItem {
	var items []RSSItem

	for i, cfg := range feedConfigs {
		feed, err := fetch(cfg.URL)
		if err != nil {
			log.Printf("[rss] error fetching %s: %v", cfg.URL, err)
		} else {
			for _, item := range feed.Items {
				ri := RSSItem{
					FeedTitle: feed.Title,
					Title:     item.Title,
					Link:      item.Link,
					Content:   item.Content,
				}
				if ri.Content == "" {
					ri.Content = item.Description
				}
				if item.PublishedParsed != nil {
					ri.Published = *item.PublishedParsed
				} else if item.UpdatedParsed != nil {
					ri.Published = *item.UpdatedParsed
				}
				items = append(items, ri)
			}
		}

		if i < len(feedConfigs)-1 {
			log.Printf("[rss] waiting %s before next feed", pause)
			time.Sleep(pause)
		}
	}

	return items
}
