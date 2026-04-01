// Package testgotoolchain provides a test Go downloads API server for e2e tests.
package testgotoolchain

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
)

// GoRelease represents a Go release version.
type GoRelease struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

// Server is a test Go downloads API server.
type Server struct {
	httpServer *httptest.Server
	mu         sync.RWMutex
	releases   []GoRelease
}

// New creates a new test gotoolchain server.
func New() *Server {
	s := &Server{
		releases: []GoRelease{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

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

// AddRelease adds a Go release to the server.
func (s *Server) AddRelease(version string, stable bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releases = append(s.releases, GoRelease{
		Version: version,
		Stable:  stable,
	})
}

// SetReleases sets all Go releases at once.
func (s *Server) SetReleases(releases []GoRelease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releases = releases
}

// handleRequest handles all API requests.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Check for ?mode=json query parameter
	if r.URL.Query().Get("mode") != "json" {
		http.NotFound(w, r)
		return
	}

	s.mu.RLock()
	releases := s.releases
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(releases); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
