package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ====================================================================
// truncate
// ====================================================================

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{"shorter than limit", "hello", 10, "hello"},
		{"equal to limit", "hello", 5, "hello"},
		{"longer than limit", "hello world", 5, "hello…"},
		{"empty string", "", 5, ""},
		{"multibyte runes", "héllo", 3, "hél…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q; want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

// ====================================================================
// extractTweetURLs
// ====================================================================

func TestExtractTweetURLs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			"no URLs",
			"no links here",
			nil,
		},
		{
			"twitter.com URL",
			"check this out https://twitter.com/user/status/123456789",
			[]string{"https://twitter.com/user/status/123456789"},
		},
		{
			"x.com URL",
			"see https://x.com/someone/status/987654321",
			[]string{"https://x.com/someone/status/987654321"},
		},
		{
			"duplicates deduplicated",
			"https://twitter.com/user/status/111 and again https://twitter.com/user/status/111",
			[]string{"https://twitter.com/user/status/111"},
		},
		{
			"multiple unique URLs",
			"https://twitter.com/a/status/1 https://twitter.com/b/status/2",
			[]string{"https://twitter.com/a/status/1", "https://twitter.com/b/status/2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTweetURLs(tt.content)
			if len(got) != len(tt.want) {
				t.Fatalf("extractTweetURLs(%q) = %v; want %v", tt.content, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] got %q; want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ====================================================================
// nextRunAt
// ====================================================================

func TestNextRunAt(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		hour int
		min  int
		want time.Time
	}{
		{
			"before target time — same day",
			time.Date(2024, 1, 15, 4, 0, 0, 0, time.UTC),
			5, 0,
			time.Date(2024, 1, 15, 5, 0, 0, 0, time.UTC),
		},
		{
			"after target time — next day",
			time.Date(2024, 1, 15, 6, 0, 0, 0, time.UTC),
			5, 0,
			time.Date(2024, 1, 16, 5, 0, 0, 0, time.UTC),
		},
		{
			"exactly at target time — next day",
			time.Date(2024, 1, 15, 5, 0, 0, 0, time.UTC),
			5, 0,
			time.Date(2024, 1, 16, 5, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextRunAt(tt.now, tt.hour, tt.min)
			if !got.Equal(tt.want) {
				t.Errorf("nextRunAt(%v, %d, %d) = %v; want %v", tt.now, tt.hour, tt.min, got, tt.want)
			}
		})
	}
}

// ====================================================================
// buildEmailHTML
// ====================================================================

func TestBuildEmailHTML(t *testing.T) {
	items := []RSSItem{
		{
			FeedTitle: "Test Feed",
			Title:     "Test Article",
			Link:      "https://example.com/article",
			Content:   "<p>Test content</p>",
			Published: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	html, err := buildEmailHTML(items, now)
	if err != nil {
		t.Fatalf("buildEmailHTML error: %v", err)
	}

	for _, want := range []string{
		"Test Feed",
		"Test Article",
		"https://example.com/article",
		"Test content",
		"1 Jun 2024",
		"1 new item(s)",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestBuildEmailHTML_MultipleFeeds(t *testing.T) {
	items := []RSSItem{
		{FeedTitle: "Feed A", Title: "Item 1", Link: "https://a.com/1"},
		{FeedTitle: "Feed B", Title: "Item 2", Link: "https://b.com/2"},
		{FeedTitle: "Feed A", Title: "Item 3", Link: "https://a.com/3"},
	}
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	html, err := buildEmailHTML(items, now)
	if err != nil {
		t.Fatalf("buildEmailHTML error: %v", err)
	}

	for _, want := range []string{"Feed A", "Feed B", "Item 1", "Item 2", "Item 3", "3 new item(s)"} {
		if !strings.Contains(html, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestBuildEmailHTML_Empty(t *testing.T) {
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	html, err := buildEmailHTML(nil, now)
	if err != nil {
		t.Fatalf("buildEmailHTML error: %v", err)
	}
	if !strings.Contains(html, "0 new item(s)") {
		t.Errorf("expected '0 item(s)' in empty digest, got:\n%s", html)
	}
}

// ====================================================================
// Store
// ====================================================================

func TestStore_SetGet(t *testing.T) {
	s := &Store{}

	items, updated := s.Get()
	if len(items) != 0 {
		t.Errorf("initial items = %v; want empty", items)
	}
	if !updated.IsZero() {
		t.Errorf("initial lastUpdated = %v; want zero", updated)
	}

	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	want := []RSSItem{{Title: "a"}, {Title: "b"}}
	s.Set(want, now)

	got, gotTime := s.Get()
	if len(got) != len(want) {
		t.Fatalf("Store.Get() len = %d; want %d", len(got), len(want))
	}
	if !gotTime.Equal(now) {
		t.Errorf("Store.Get() time = %v; want %v", gotTime, now)
	}
}

// ====================================================================
// SeenStore
// ====================================================================

func TestSeenStore_IsNew_MarkSeen(t *testing.T) {
	ss := newSeenStore()
	link := "https://example.com/1"

	if !ss.IsNew(link) {
		t.Error("expected unseen link to be new")
	}

	ss.MarkSeen([]string{link})

	if ss.IsNew(link) {
		t.Error("expected link to be seen after MarkSeen")
	}
}

func TestSeenStore_Cleanup(t *testing.T) {
	ss := newSeenStore()
	ss.seen["old"] = time.Now().UTC().AddDate(0, 0, -8)
	ss.seen["recent"] = time.Now().UTC()

	ss.Cleanup()

	if !ss.IsNew("old") {
		t.Error("expected old entry to be removed by Cleanup")
	}
	if ss.IsNew("recent") {
		t.Error("expected recent entry to survive Cleanup")
	}
}

func TestSeenStore_MarkSeen_Multiple(t *testing.T) {
	ss := newSeenStore()
	links := []string{"https://a.com", "https://b.com", "https://c.com"}

	ss.MarkSeen(links)

	for _, l := range links {
		if ss.IsNew(l) {
			t.Errorf("expected %q to be seen after MarkSeen", l)
		}
	}
}

// ====================================================================
// postprocessRedditLive
// ====================================================================

func TestPostprocessRedditLive_NoTweets(t *testing.T) {
	items := []RSSItem{
		{FeedTitle: "feed", Title: "plain item", Content: "no links here"},
	}
	stub := func(feedTitle, url string) RSSItem {
		t.Errorf("fetchFn should not be called for non-tweet content")
		return RSSItem{}
	}

	result := postprocessRedditLive(items, stub)

	if len(result) != 1 || result[0].Title != "plain item" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestPostprocessRedditLive_WithTweet(t *testing.T) {
	items := []RSSItem{
		{
			FeedTitle: "Live Feed",
			Content:   "See https://twitter.com/user/status/123 for more",
		},
	}
	stub := func(feedTitle, url string) RSSItem {
		return RSSItem{
			FeedTitle: feedTitle,
			Title:     "stubbed tweet",
			Link:      url,
		}
	}

	result := postprocessRedditLive(items, stub)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Title != "stubbed tweet" {
		t.Errorf("title = %q; want 'stubbed tweet'", result[0].Title)
	}
	if result[0].Link != "https://twitter.com/user/status/123" {
		t.Errorf("link = %q; want twitter URL", result[0].Link)
	}
	if result[0].FeedTitle != "Live Feed" {
		t.Errorf("feedTitle = %q; want 'Live Feed'", result[0].FeedTitle)
	}
}

func TestPostprocessRedditLive_MultipleTweetsInOneItem(t *testing.T) {
	items := []RSSItem{
		{
			FeedTitle: "Live Feed",
			Content:   "https://twitter.com/a/status/1 and https://twitter.com/b/status/2",
		},
	}
	var called []string
	stub := func(feedTitle, url string) RSSItem {
		called = append(called, url)
		return RSSItem{Title: "tweet:" + url, Link: url}
	}

	result := postprocessRedditLive(items, stub)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if len(called) != 2 {
		t.Errorf("expected fetchFn called twice, got %d times", len(called))
	}
}

// ====================================================================
// HTTP handlers
// ====================================================================

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("body = %q; want 'ok'", w.Body.String())
	}
}

func TestRootHandler_NotFound(t *testing.T) {
	store := &Store{}
	req := httptest.NewRequest(http.MethodGet, "/other", nil)
	w := httptest.NewRecorder()

	rootHandler(store)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

func TestRootHandler_OK(t *testing.T) {
	store := &Store{}
	store.Set([]RSSItem{
		{FeedTitle: "My Feed", Title: "An Item", Link: "https://example.com"},
	}, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	rootHandler(store)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
	for _, want := range []string{"My Feed", "An Item", "https://example.com"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestRootHandler_Empty(t *testing.T) {
	store := &Store{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	rootHandler(store)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "RSS Reader") {
		t.Errorf("body missing 'RSS Reader'")
	}
}
