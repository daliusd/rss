package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
)

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
	LastUpdated interface{ IsZero() bool }
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
