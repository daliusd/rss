package main

import (
	"log"
	"sync"
	"time"
)

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
	seen map[string]time.Time
}

func newSeenStore() *SeenStore {
	return &SeenStore{seen: make(map[string]time.Time)}
}

// IsNew returns true if the link has not been seen before.
func (ss *SeenStore) IsNew(link string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.seen[link].IsZero()
}

// MarkSeen records the links as seen with the current UTC time.
func (ss *SeenStore) MarkSeen(links []string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	now := time.Now().UTC()
	for _, l := range links {
		ss.seen[l] = now
	}
}

// Cleanup removes entries that were seen more than 7 days ago.
func (ss *SeenStore) Cleanup() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	cutoff := time.Now().UTC().AddDate(0, 0, -7)
	for link, seenAt := range ss.seen {
		if seenAt.Before(cutoff) {
			delete(ss.seen, link)
		}
	}
	log.Printf("[seen] store size after cleanup: %d", len(ss.seen))
}
