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

// ReleaseVersion represents a release with its tag and publish time.
type ReleaseVersion struct {
	TagName     string
	PublishedAt time.Time
	CreatedAt   time.Time
	Draft       bool
}

// Server is a test GitHub API server.
type Server struct {
	httpServer *httptest.Server
	mu         sync.RWMutex
	repos      map[string][]TagVersion     // key: "owner/repo"
	releases   map[string][]ReleaseVersion // key: "owner/repo"
	registries map[string]string           // key: ref
}

// New creates a new test GitHub API server.
func New() *Server {
	s := &Server{
		repos:      make(map[string][]TagVersion),
		releases:   make(map[string][]ReleaseVersion),
		registries: make(map[string]string),
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

// AddReleaseRepo adds a repository with its releases to the server.
func (s *Server) AddReleaseRepo(owner, repo string, releases []ReleaseVersion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s/%s", owner, repo)
	s.releases[key] = releases
}

// AddRegistry adds a standard aqua registry fixture for the given ref.
func (s *Server) AddRegistry(ref, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registries[ref] = content
}

// handleRequest handles all API requests.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	if strings.HasPrefix(path, "raw/aquaproj/aqua-registry/") {
		s.handleRegistry(w, r, path)
		return
	}

	// Parse the request path
	// Format: repos/<owner>/<repo>/tags, /releases, or /commits/<sha>
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
	releases := s.releases[key]
	s.mu.RUnlock()

	if !ok && len(releases) == 0 {
		http.NotFound(w, r)
		return
	}

	if parts[3] == "tags" {
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.handleTags(w, tags)
		return
	}

	if parts[3] == "releases" {
		s.handleReleases(w, releases)
		return
	}

	if parts[3] == "commits" && len(parts) == 5 {
		if !ok {
			http.NotFound(w, r)
			return
		}
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

// handleReleases handles /repos/{owner}/{repo}/releases requests.
func (s *Server) handleReleases(w http.ResponseWriter, releases []ReleaseVersion) {
	w.Header().Set("Content-Type", "application/json")

	type releaseResponse struct {
		TagName     string    `json:"tag_name"`
		PublishedAt time.Time `json:"published_at,omitempty"`
		CreatedAt   time.Time `json:"created_at,omitempty"`
		Draft       bool      `json:"draft"`
	}

	var response []releaseResponse
	for _, release := range releases {
		response = append(response, releaseResponse(release))
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

func (s *Server) handleRegistry(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(path, "/")
	if len(parts) != 5 || parts[4] != "registry.yaml" {
		http.NotFound(w, r)
		return
	}

	ref := parts[3]

	s.mu.RLock()
	content, ok := s.registries[ref]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	_, _ = w.Write([]byte(content))
}
