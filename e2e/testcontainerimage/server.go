// Package testcontainerimage provides a test container image registry server for e2e tests.
package testcontainerimage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// TagEntry represents a Docker image tag.
type TagEntry struct {
	Name        string    `json:"name"`
	LastUpdated time.Time `json:"last_updated"`
}

// tagResponse represents the Docker Hub API response for tags.
type tagResponse struct {
	Results []TagEntry `json:"results"`
	Next    string     `json:"next"`
}

// Server is a test Docker Hub API server.
type Server struct {
	httpServer *httptest.Server
	mu         sync.RWMutex
	repos      map[string][]TagEntry // key: "owner/repo"
}

// New creates a new test container image registry server.
func New() *Server {
	s := &Server{
		repos: make(map[string][]TagEntry),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/repositories/", s.handleTags)

	s.httpServer = httptest.NewServer(mux)
	return s
}

// URL returns the server's URL.
func (s *Server) URL() string {
	return s.httpServer.URL
}

// Close shuts down the server.
func (s *Server) Close() {
	s.httpServer.Close()
}

// AddRepo adds a repository with its tags to the server.
func (s *Server) AddRepo(repository string, tags []TagEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repos[repository] = tags
}

// handleTags handles tag list requests.
func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	// Extract repository from path: /v2/repositories/{repo}/tags
	// Path format: /v2/repositories/library/debian/tags
	path := r.URL.Path
	const prefix = "/v2/repositories/"
	const suffix = "/tags"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		http.NotFound(w, r)
		return
	}

	repo := path[len(prefix) : len(path)-len(suffix)]
	if repo == "" {
		http.NotFound(w, r)
		return
	}

	s.mu.RLock()
	tags, ok := s.repos[repo]
	s.mu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	resp := tagResponse{
		Results: tags,
		Next:    "",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
