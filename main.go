package main

import (
	"log"
	"net/http"
)

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
