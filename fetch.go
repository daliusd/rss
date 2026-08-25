package main

import (
	"log"
	"sync"

	"github.com/mmcdole/gofeed"
)

// ====================================================================
// Feed configuration
// ====================================================================

var feedConfigs = []FeedConfig{
	{URL: "https://addyosmani.com/rss.xml"},
	{URL: "https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_anthropic_engineering.xml"},
	{URL: "https://www.reddit.com/r/neovim/top/.rss?t=week"},
	{URL: "https://www.youtube.com/feeds/videos.xml?channel_id=UCswG6FSbgZjbWtdf_hMLaow"},
	{URL: "https://www.youtube.com/feeds/videos.xml?channel_id=UCLKPca3kwwd-B59HNr-_lvA"},
}

// ====================================================================
// RSS fetch logic
// ====================================================================

func fetchFeeds() []RSSItem {
	parser := gofeed.NewParser()
	parser.UserAgent = "rss-reader/1.0"

	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		items []RSSItem
	)

	for _, cfg := range feedConfigs {
		wg.Add(1)
		go func(cfg FeedConfig) {
			defer wg.Done()
			feed, err := parser.ParseURL(cfg.URL)
			if err != nil {
				log.Printf("[rss] error fetching %s: %v", cfg.URL, err)
				return
			}
			feedTitle := feed.Title

			var local []RSSItem
			for _, item := range feed.Items {
				ri := RSSItem{
					FeedTitle: feedTitle,
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
				local = append(local, ri)
			}
			mu.Lock()
			items = append(items, local...)
			mu.Unlock()
		}(cfg)
	}

	wg.Wait()
	return items
}
