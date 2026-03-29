// Package testgithub provides a test GitHub API server for e2e tests.
package testgithub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// TagVersion represents a tag with its commit SHA and date.
type TagVersion struct {
	Name       string
	CommitSHA  string
	CommitDate time.Time
}

// Server is a test GitHub API server.
type Server struct {
	httpServer *httptest.Server
	mu         sync.RWMutex
	repos      map[string][]TagVersion // key: "owner/repo"
}

// New creates a new test GitHub API server.
func New() *Server {
	s := &Server{
		repos: make(map[string][]TagVersion),
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

// AddRepo adds a repository with its tags to the server.
func (s *Server) AddRepo(owner, repo string, tags []TagVersion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s/%s", owner, repo)
	s.repos[key] = tags
}

// handleRequest handles all API requests.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	// Parse the request path
	// Format: repos/<owner>/<repo>/tags or repos/<owner>/<repo>/commits/<sha>
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[0] != "repos" {
		http.NotFound(w, r)
		return
	}

	owner := parts[1]
	repo := parts[2]
	key := fmt.Sprintf("%s/%s", owner, repo)

	s.mu.RLock()
	tags, ok := s.repos[key]
	s.mu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	if parts[3] == "tags" {
		s.handleTags(w, tags)
		return
	}

	if parts[3] == "commits" && len(parts) == 5 {
		sha := parts[4]
		s.handleCommit(w, tags, sha)
		return
	}

	http.NotFound(w, r)
}

// handleTags handles /repos/{owner}/{repo}/tags requests.
func (s *Server) handleTags(w http.ResponseWriter, tags []TagVersion) {
	w.Header().Set("Content-Type", "application/json")

	type commitRef struct {
		SHA string `json:"sha"`
		URL string `json:"url"`
	}
	type tagResponse struct {
		Name   string    `json:"name"`
		Commit commitRef `json:"commit"`
	}

	var response []tagResponse
	for _, tag := range tags {
		response = append(response, tagResponse{
			Name: tag.Name,
			Commit: commitRef{
				SHA: tag.CommitSHA,
				URL: fmt.Sprintf("/commits/%s", tag.CommitSHA),
			},
		})
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleCommit handles /repos/{owner}/{repo}/commits/{sha} requests.
func (s *Server) handleCommit(w http.ResponseWriter, tags []TagVersion, sha string) {
	w.Header().Set("Content-Type", "application/json")

	// Find the tag with this SHA
	for _, tag := range tags {
		if tag.CommitSHA == sha {
			response := struct {
				SHA    string `json:"sha"`
				Commit struct {
					Committer struct {
						Date time.Time `json:"date"`
					} `json:"committer"`
				} `json:"commit"`
			}{
				SHA: sha,
			}
			response.Commit.Committer.Date = tag.CommitDate

			if err := json.NewEncoder(w).Encode(response); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}

	http.NotFound(w, nil)
}
