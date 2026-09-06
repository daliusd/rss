package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
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

// redditCooldownBuffer is added to any cooldown Reddit asks for, so we come
// back a little after the quota has actually reset rather than right on it.
const redditCooldownBuffer = 5 * time.Second

func redditCooldown(headers http.Header) time.Duration {
	if retryAfter, ok := headerSeconds(headers, "Retry-After"); ok {
		return retryAfter + redditCooldownBuffer
	}

	remaining, ok := headerSeconds(headers, "X-Ratelimit-Remaining")
	if !ok || remaining > 0 {
		return 0
	}

	reset, ok := headerSeconds(headers, "X-Ratelimit-Reset")
	if !ok {
		return 0
	}
	return reset + redditCooldownBuffer
}

func logRedditRateLimit(feedURL string, headers http.Header) {
	log.Printf(
		"[rss] Reddit rate limit for %s: remaining=%s used=%s reset=%s retry-after=%s",
		feedURL,
		headerOrDash(headers, "X-Ratelimit-Remaining"),
		headerOrDash(headers, "X-Ratelimit-Used"),
		headerOrDash(headers, "X-Ratelimit-Reset"),
		headerOrDash(headers, "Retry-After"),
	)
}

func headerOrDash(headers http.Header, name string) string {
	if value := headers.Get(name); value != "" {
		return value
	}
	return "-"
}

func headerSeconds(headers http.Header, name string) (time.Duration, bool) {
	seconds, err := strconv.ParseFloat(headers.Get(name), 64)
	if err != nil || seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

func fetchFeeds() []RSSItem {
	return fetchFeedsWithHeaders(fetchFeed, feedFetchPause, time.Now, time.Sleep)
}

func fetchFeed(feedURL string) (*gofeed.Feed, http.Header, error) {
	request, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("User-Agent", "rss-reader/1.0")

	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, response.Header, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}

	parser := gofeed.NewParser()
	feed, err := parser.Parse(response.Body)
	return feed, response.Header, err
}

// fetchFeedsWith is the sequential fetch implementation, with its network
// operation and pause injectable for testing.
func fetchFeedsWith(fetch func(string) (*gofeed.Feed, error), pause time.Duration) []RSSItem {
	return fetchFeedsWithHeaders(
		func(feedURL string) (*gofeed.Feed, http.Header, error) {
			feed, err := fetch(feedURL)
			return feed, nil, err
		},
		pause,
		time.Now,
		time.Sleep,
	)
}

func fetchFeedsWithHeaders(
	fetch func(string) (*gofeed.Feed, http.Header, error),
	pause time.Duration,
	now func() time.Time,
	sleep func(time.Duration),
) []RSSItem {
	var items []RSSItem
	var nextRedditFetchAt time.Time

	for i, cfg := range feedConfigs {
		if isRedditFeed(cfg.URL) && now().Before(nextRedditFetchAt) {
			cooldown := nextRedditFetchAt.Sub(now())
			log.Printf("[rss] waiting %s for Reddit rate limit", cooldown)
			sleep(cooldown)
		}

		feed, headers, err := fetch(cfg.URL)
		if isRedditFeed(cfg.URL) {
			logRedditRateLimit(cfg.URL, headers)
			if cooldown := redditCooldown(headers); cooldown > 0 {
				log.Printf("[rss] Reddit cooldown of %s before next Reddit feed", cooldown)
				allowedAt := now().Add(cooldown)
				if allowedAt.After(nextRedditFetchAt) {
					nextRedditFetchAt = allowedAt
				}
			}
		}
		if err != nil {
			log.Printf("[rss] error fetching %s: %v", cfg.URL, err)
		} else {
			log.Printf("[rss] fetched %s: %d items", cfg.URL, len(feed.Items))
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
			sleep(pause)
		}
	}

	return items
}

func isRedditFeed(feedURL string) bool {
	parsed, err := url.Parse(feedURL)
	return err == nil && parsed.Hostname() == "www.reddit.com"
}
