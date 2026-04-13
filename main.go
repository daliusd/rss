package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/smtp"
	"os"
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
// Seen-items store (in-memory)
// ====================================================================

type SeenStore struct {
	mu   sync.Mutex
	seen map[string]bool
}

func newSeenStore() *SeenStore {
	return &SeenStore{seen: make(map[string]bool)}
}

// IsNew returns true if the link has not been seen before.
func (ss *SeenStore) IsNew(link string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return !ss.seen[link]
}

// MarkSeen records the links as seen.
func (ss *SeenStore) MarkSeen(links []string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	for _, l := range links {
		ss.seen[l] = true
	}
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
// Email
// ====================================================================

const (
	mailTo       = "dalius.dobravolskas@gmail.com"
	smtpHost     = "smtp.gmail.com"
	smtpPort     = 587
)

var emailTmpl = template.Must(template.New("email").Funcs(template.FuncMap{
	"safeHTML": func(s string) template.HTML { return template.HTML(s) },
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>RSS Digest</title>
  <style>
    body { font-family: system-ui, sans-serif; background: #f5f5f5; color: #222; padding: 1rem; }
    h1 { font-size: 1.5rem; margin-bottom: 0.5rem; }
    h2 { font-size: 1.1rem; border-bottom: 2px solid #ddd; padding-bottom: 0.3rem; margin: 1.5rem 0 0.75rem; }
    .item { background: #fff; border: 1px solid #e0e0e0; border-radius: 6px; padding: 1rem; margin-bottom: 0.75rem; }
    .item h3 { font-size: 0.95rem; margin-bottom: 0.3rem; }
    .item h3 a { color: #0066cc; text-decoration: none; }
    .meta { font-size: 0.75rem; color: #888; margin-bottom: 0.5rem; }
    .content { font-size: 0.875rem; line-height: 1.6; }
    .content img { max-width: 100%; height: auto; }
  </style>
</head>
<body>
  <h1>RSS Digest — {{.Date}}</h1>
  <p>{{.Total}} new item(s) across {{len .Feeds}} feed(s).</p>
  {{range .Feeds}}
  <h2>{{.Name}}</h2>
  {{range .Items}}
  <div class="item">
    <h3><a href="{{.Link}}">{{.Title}}</a></h3>
    {{if not .Published.IsZero}}<p class="meta">{{.Published.Format "2 Jan 2006"}}</p>{{end}}
    {{if .Content}}<div class="content">{{safeHTML .Content}}</div>{{end}}
  </div>
  {{end}}
  {{end}}
</body>
</html>
`))

type emailFeedGroup struct {
	Name  string
	Items []RSSItem
}

type emailData struct {
	Date  string
	Total int
	Feeds []emailFeedGroup
}

func buildEmailHTML(newItems []RSSItem) (string, error) {
	// Group by feed, preserving order of first appearance.
	orderMap := map[string]int{}
	var groups []emailFeedGroup
	for _, item := range newItems {
		idx, ok := orderMap[item.FeedTitle]
		if !ok {
			idx = len(groups)
			orderMap[item.FeedTitle] = idx
			groups = append(groups, emailFeedGroup{Name: item.FeedTitle})
		}
		groups[idx].Items = append(groups[idx].Items, item)
	}

	data := emailData{
		Date:  time.Now().UTC().Format("2 Jan 2006"),
		Total: len(newItems),
		Feeds: groups,
	}

	var buf bytes.Buffer
	if err := emailTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render email template: %w", err)
	}
	return buf.String(), nil
}

func sendEmail(subject, htmlBody string) error {
	user := os.Getenv("MAIL_USER")
	pass := os.Getenv("MAIL_PASS")
	if user == "" || pass == "" {
		return fmt.Errorf("MAIL_USER or MAIL_PASS env variable is not set")
	}

	auth := smtp.PlainAuth("", user, pass, smtpHost)
	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)

	// Build a minimal RFC 2822 + MIME message with HTML body.
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\n", user)
	fmt.Fprintf(&msg, "To: %s\r\n", mailTo)
	fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: text/html; charset=\"UTF-8\"\r\n")
	fmt.Fprintf(&msg, "\r\n")
	msg.WriteString(htmlBody)

	return smtp.SendMail(addr, auth, user, []string{mailTo}, msg.Bytes())
}

// ====================================================================
// Scheduler
// ====================================================================

func runDailyJob(store *Store, seen *SeenStore) {
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

	htmlBody, err := buildEmailHTML(newItems)
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

func scheduler(store *Store, seen *SeenStore) {
	runDailyJob(store, seen)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		runDailyJob(store, seen)
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
	seen := newSeenStore()

	go scheduler(store, seen)

	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler(store))
	mux.HandleFunc("/health", healthHandler)

	addr := ":8080"
	log.Printf("[http] listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[http] server error: %v", err)
	}
}
