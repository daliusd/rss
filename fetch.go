package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
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
	{URL: "https://www.reddit.com/live/18hnzysb1elcs.rss", Postprocess: func(items []RSSItem) []RSSItem {
		return postprocessRedditLive(items, fetchTweet)
	}},
	{URL: "https://www.aihero.dev/rss.xml"},
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

			if cfg.Postprocess != nil {
				local = cfg.Postprocess(local)
			}

			mu.Lock()
			items = append(items, local...)
			mu.Unlock()
		}(cfg)
	}

	wg.Wait()
	return items
}

// ====================================================================
// Twitter / fxtwitter post-processing
// ====================================================================

var (
	twitterURLRe = regexp.MustCompile(`https?://(?:www\.)?(?:twitter\.com|x\.com)/(\w+)/status/(\d+)`)
	fxClient     = &http.Client{Timeout: 10 * time.Second}
)

type fxResponse struct {
	Code  int     `json:"code"`
	Tweet fxTweet `json:"tweet"`
}

type fxTweet struct {
	URL       string   `json:"url"`
	Text      string   `json:"text"`
	Author    fxAuthor `json:"author"`
	CreatedAt int64    `json:"created_timestamp"`
	Media     *fxMedia `json:"media"`
}

type fxAuthor struct {
	Name       string `json:"name"`
	ScreenName string `json:"screen_name"`
}

type fxMedia struct {
	Photos []fxPhoto `json:"photos"`
}

type fxPhoto struct {
	URL string `json:"url"`
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

func stubTweet(feedTitle, tweetURL string) RSSItem {
	return RSSItem{
		FeedTitle: feedTitle,
		Title:     tweetURL,
		Link:      tweetURL,
		Content:   fmt.Sprintf(`<a href="%s">%s</a>`, tweetURL, tweetURL),
	}
}

func fetchTweet(feedTitle, tweetURL string) RSSItem {
	m := twitterURLRe.FindStringSubmatch(tweetURL)
	if m == nil {
		return stubTweet(feedTitle, tweetURL)
	}
	username, id := m[1], m[2]
	apiURL := fmt.Sprintf("https://api.fxtwitter.com/%s/status/%s", username, id)

	resp, err := fxClient.Get(apiURL)
	if err != nil {
		log.Printf("[fxtwitter] fetch error for %s: %v", tweetURL, err)
		return stubTweet(feedTitle, tweetURL)
	}
	defer resp.Body.Close()

	var fx fxResponse
	if err := json.NewDecoder(resp.Body).Decode(&fx); err != nil || fx.Code != 200 {
		log.Printf("[fxtwitter] decode error for %s: %v (code %d)", tweetURL, err, fx.Code)
		return stubTweet(feedTitle, tweetURL)
	}

	t := fx.Tweet
	title := fmt.Sprintf("@%s: %s", t.Author.ScreenName, truncate(t.Text, 80))

	var sb strings.Builder
	sb.WriteString(t.Text)
	if t.Media != nil {
		for _, p := range t.Media.Photos {
			fmt.Fprintf(&sb, `<br><img src="%s" alt="tweet image">`, p.URL)
		}
	}

	var published time.Time
	if t.CreatedAt != 0 {
		published = time.Unix(t.CreatedAt, 0).UTC()
	}

	return RSSItem{
		FeedTitle: feedTitle,
		Title:     title,
		Link:      tweetURL,
		Content:   sb.String(),
		Published: published,
	}
}

func extractTweetURLs(content string) []string {
	matches := twitterURLRe.FindAllString(content, -1)
	seen := map[string]bool{}
	var urls []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			urls = append(urls, m)
		}
	}
	return urls
}

func postprocessRedditLive(items []RSSItem, fetchFn func(feedTitle, tweetURL string) RSSItem) []RSSItem {
	var result []RSSItem
	for _, item := range items {
		links := extractTweetURLs(item.Content)
		if len(links) == 0 {
			result = append(result, item)
			continue
		}
		for _, tweetURL := range links {
			result = append(result, fetchFn(item.FeedTitle, tweetURL))
		}
	}
	return result
}
