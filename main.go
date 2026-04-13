package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// ====================================================================
// Data types
// ====================================================================

// DailySnapshot is whatever your cron job computes once every 24 hours.
// Replace the fields below with whatever you actually want to store.
type DailySnapshot struct {
	ComputedAt  time.Time `json:"computed_at"`
	Message     string    `json:"message"`
	RandomValue int       `json:"random_value"` // placeholder for real work
}

// ====================================================================
// In-memory store (safe for concurrent access)
// ====================================================================

type Store struct {
	mu       sync.RWMutex
	snapshot *DailySnapshot
}

func (s *Store) Set(snap DailySnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = &snap
}

func (s *Store) Get() *DailySnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot
}

// ====================================================================
// The work that runs every 24 hours
// ====================================================================

func runDailyJob(store *Store) {
	snap := DailySnapshot{
		ComputedAt:  time.Now().UTC(),
		Message:     "Daily job ran successfully",
		RandomValue: rand.Intn(1_000_000), // replace with your real logic
	}
	store.Set(snap)
	log.Printf("[cron] snapshot updated at %s", snap.ComputedAt.Format(time.RFC3339))
}

// scheduler runs the job immediately on startup, then every 24 hours.
func scheduler(store *Store) {
	runDailyJob(store) // run once right away so the store is never empty

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		runDailyJob(store)
	}
}

// ====================================================================
// HTTP handlers
// ====================================================================

func snapshotHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := store.Get()
		if snap == nil {
			http.Error(w, "snapshot not yet available", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(snap); err != nil {
			log.Printf("[http] encode error: %v", err)
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

	// Start the background scheduler in its own goroutine.
	go scheduler(store)

	// Register routes.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/snapshot", snapshotHandler(store))

	addr := ":8080"
	log.Printf("[http] listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[http] server error: %v", err)
	}
}
