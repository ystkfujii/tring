// Package testproxy provides a test Go module proxy server for e2e tests.
package testproxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// ModuleVersion represents a version with its release time.
type ModuleVersion struct {
	Version    string
	ReleasedAt time.Time
}

// Server is a test Go module proxy server.
type Server struct {
	httpServer *httptest.Server
	mu         sync.RWMutex
	modules    map[string][]ModuleVersion
}

// New creates a new test proxy server.
func New() *Server {
	s := &Server{
		modules: make(map[string][]ModuleVersion),
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

// AddModule adds a module with its versions to the proxy.
func (s *Server) AddModule(modulePath string, versions []ModuleVersion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modules[modulePath] = versions
}

// handleRequest handles all proxy requests.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	// Parse the request path
	// Format: <escaped-module>/@v/list or <escaped-module>/@v/<version>.info
	atVIndex := strings.Index(path, "/@v/")
	if atVIndex == -1 {
		http.NotFound(w, r)
		return
	}

	escapedModule := path[:atVIndex]
	versionPart := path[atVIndex+4:] // After "/@v/"

	// Unescape the module path
	modulePath := unescapePath(escapedModule)

	s.mu.RLock()
	versions, ok := s.modules[modulePath]
	s.mu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	if versionPart == "list" {
		s.handleList(w, versions)
		return
	}

	if strings.HasSuffix(versionPart, ".info") {
		version := strings.TrimSuffix(versionPart, ".info")
		s.handleInfo(w, r, versions, version)
		return
	}

	http.NotFound(w, r)
}

// handleList handles /@v/list requests.
func (s *Server) handleList(w http.ResponseWriter, versions []ModuleVersion) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, v := range versions {
		fmt.Fprintln(w, v.Version)
	}
}

// handleInfo handles /@v/<version>.info requests.
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request, versions []ModuleVersion, version string) {
	for _, v := range versions {
		if v.Version == version {
			w.Header().Set("Content-Type", "application/json")
			info := struct {
				Version string    `json:"Version"`
				Time    time.Time `json:"Time"`
			}{
				Version: v.Version,
				Time:    v.ReleasedAt,
			}
			if err := json.NewEncoder(w).Encode(info); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}
	http.NotFound(w, r)
}

// unescapePath unescapes a module path from the proxy URL format.
// In proxy URLs, uppercase letters in module paths are escaped as '!' followed by lowercase.
func unescapePath(escaped string) string {
	var b strings.Builder
	i := 0
	for i < len(escaped) {
		if escaped[i] == '!' && i+1 < len(escaped) {
			// Convert !x to X
			b.WriteByte(escaped[i+1] - ('a' - 'A'))
			i += 2
		} else {
			b.WriteByte(escaped[i])
			i++
		}
	}
	return b.String()
}
