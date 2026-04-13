package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
)

// ====================================================================
// Data types
// ====================================================================

var feedURLs = []string{
	"https://addyosmani.com/rss.xml",
	"https://raw.githubusercontent.com/Olshansk/rss-feeds/main/feeds/feed_anthropic_engineering.xml",
	"https://www.reddit.com/r/neovim/top/.rss?t=week",
}

type RSSItem struct {
	FeedTitle string
	Title     string
	Link      string
	Content   string
	Published time.Time
}

// ====================================================================
// In-memory store (safe for concurrent access)
// ====================================================================

type Store struct {
	mu          sync.RWMutex
	items       []RSSItem
	lastUpdated time.Time
}

func (s *Store) Set(items []RSSItem, updatedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = items
	s.lastUpdated = updatedAt
}

func (s *Store) Get() ([]RSSItem, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.items, s.lastUpdated
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

	for _, url := range feedURLs {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			feed, err := parser.ParseURL(u)
			if err != nil {
				log.Printf("[rss] error fetching %s: %v", u, err)
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
		}(url)
	}

	wg.Wait()
	return items
}

// ====================================================================
// Scheduler
// ====================================================================

func runDailyJob(store *Store) {
	log.Println("[rss] fetching feeds…")
	items := fetchFeeds()
	now := time.Now().UTC()
	store.Set(items, now)
	log.Printf("[rss] fetched %d items at %s", len(items), now.Format(time.RFC3339))
}

func scheduler(store *Store) {
	runDailyJob(store)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		runDailyJob(store)
	}
}

// ====================================================================
// HTML template
// ====================================================================

var pageTmpl = template.Must(template.New("page").Funcs(template.FuncMap{
	"safeHTML": func(s string) template.HTML { return template.HTML(s) },
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RSS Reader</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: system-ui, sans-serif; background: #f5f5f5; color: #222; padding: 2rem 1rem; }
    header { max-width: 860px; margin: 0 auto 2rem; }
    header h1 { font-size: 1.8rem; }
    header p  { color: #666; font-size: 0.875rem; margin-top: 0.25rem; }
    .feed { max-width: 860px; margin: 0 auto 3rem; }
    .feed h2 { font-size: 1.2rem; border-bottom: 2px solid #ddd; padding-bottom: 0.4rem; margin-bottom: 1rem; }
    .item { background: #fff; border: 1px solid #e0e0e0; border-radius: 6px; padding: 1.25rem; margin-bottom: 1rem; }
    .item h3 { font-size: 1rem; margin-bottom: 0.4rem; }
    .item h3 a { color: #0066cc; text-decoration: none; }
    .item h3 a:hover { text-decoration: underline; }
    .item .meta { font-size: 0.75rem; color: #888; margin-bottom: 0.75rem; }
    .item .content { font-size: 0.9rem; line-height: 1.6; }
    .item .content img { max-width: 100%; height: auto; }
    .empty { color: #999; font-style: italic; }
  </style>
</head>
<body>
  <header>
    <h1>RSS Reader</h1>
    <p>Last updated: {{if .LastUpdated.IsZero}}fetching…{{else}}{{.LastUpdated.Format "2006-01-02 15:04 UTC"}}{{end}}</p>
  </header>

  {{range .Feeds}}
  <section class="feed">
    <h2>{{.Name}}</h2>
    {{if not .Items}}<p class="empty">No items.</p>{{end}}
    {{range .Items}}
    <article class="item">
      <h3><a href="{{.Link}}" target="_blank" rel="noopener">{{.Title}}</a></h3>
      {{if not .Published.IsZero}}<p class="meta">{{.Published.Format "2 Jan 2006"}}</p>{{end}}
      {{if .Content}}<div class="content">{{safeHTML .Content}}</div>{{end}}
    </article>
    {{end}}
  </section>
  {{end}}
</body>
</html>
`))

type feedGroup struct {
	Name  string
	Items []RSSItem
}

type pageData struct {
	LastUpdated time.Time
	Feeds       []feedGroup
}

// ====================================================================
// HTTP handlers
// ====================================================================

func rootHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		items, lastUpdated := store.Get()

		// Group by feed title, preserving order of first appearance.
		orderMap := map[string]int{}
		var groups []feedGroup
		for _, item := range items {
			idx, ok := orderMap[item.FeedTitle]
			if !ok {
				idx = len(groups)
				orderMap[item.FeedTitle] = idx
				groups = append(groups, feedGroup{Name: item.FeedTitle})
			}
			groups[idx].Items = append(groups[idx].Items, item)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pageTmpl.Execute(w, pageData{LastUpdated: lastUpdated, Feeds: groups}); err != nil {
			log.Printf("[http] template error: %v", err)
		}
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, `{"status":"ok"}`)
}

// ====================================================================
// Main
// ====================================================================

func main() {
	store := &Store{}

	go scheduler(store)

	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler(store))
	mux.HandleFunc("/health", healthHandler)

	addr := ":8080"
	log.Printf("[http] listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[http] server error: %v", err)
	}
}
