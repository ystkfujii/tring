package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/e2e/testgithub"
	"github.com/ystkfujii/tring/e2e/testproxy"
	"github.com/ystkfujii/tring/internal/app/apply"
	"github.com/ystkfujii/tring/internal/config"
	"github.com/ystkfujii/tring/pkg/impl/sources"

	// Register source implementations
	_ "github.com/ystkfujii/tring/pkg/impl/sources/envfile"
	_ "github.com/ystkfujii/tring/pkg/impl/sources/githubaction"
	_ "github.com/ystkfujii/tring/pkg/impl/sources/gomod"

	// Register resolver implementations
	_ "github.com/ystkfujii/tring/pkg/impl/resolver/githubrelease"
	_ "github.com/ystkfujii/tring/pkg/impl/resolver/goproxy"
)

type versionsFixture struct {
	ProxyModules map[string][]proxyVersionFixture   `yaml:"proxy_modules"`
	GitHubRepos  map[string][]githubTagVersionEntry `yaml:"github_repos"`
}

type proxyVersionsFixture struct {
	ProxyModules map[string][]proxyVersionFixture `yaml:"proxy_modules"`
}

type githubVersionsFixture struct {
	GitHubRepos map[string][]githubTagVersionEntry `yaml:"github_repos"`
}

type proxyVersionFixture struct {
	Version    string `yaml:"version"`
	ReleasedAt string `yaml:"released_at"`
}

type githubTagVersionEntry struct {
	Name       string `yaml:"name"`
	CommitSHA  string `yaml:"commit_sha"`
	CommitDate string `yaml:"commit_date"`
}

func TestApplyE2E(t *testing.T) {
	proxyFixture, err := loadProxyVersionFixtures(filepath.Join("testdata", "versions.goproxy.yaml"))
	if err != nil {
		t.Fatalf("failed to load goproxy version fixtures: %v", err)
	}

	githubFixture, err := loadGitHubVersionFixtures(filepath.Join("testdata", "versions.githubrelease.yaml"))
	if err != nil {
		t.Fatalf("failed to load githubrelease version fixtures: %v", err)
	}

	fixtures := &versionsFixture{
		ProxyModules: proxyFixture.ProxyModules,
		GitHubRepos:  githubFixture.GitHubRepos,
	}

	// Start test servers
	proxy := testproxy.New()
	defer proxy.Close()
	if err := setupTestModules(proxy, fixtures); err != nil {
		t.Fatalf("failed to setup proxy modules: %v", err)
	}

	github := testgithub.New()
	defer github.Close()
	if err := setupTestRepos(github, fixtures); err != nil {
		t.Fatalf("failed to setup github repos: %v", err)
	}

	// Build substitutions map for all placeholders
	substitutions := map[string]string{
		"{{PROXY_URL}}": proxy.URL(),
		"{{API_URL}}":   github.URL(),
	}

	// Discover test cases from testdata directory
	testCases, err := discoverTestCases("testdata")
	if err != nil {
		t.Fatalf("failed to discover test cases: %v", err)
	}

	for _, caseName := range testCases {
		t.Run(caseName, func(t *testing.T) {
			runTestCase(t, caseName, substitutions)
		})
	}
}

func loadProxyVersionFixtures(path string) (*proxyVersionsFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var fixture proxyVersionsFixture
	if err := yaml.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("failed to parse goproxy fixture: %w", err)
	}

	return &fixture, nil
}

func loadGitHubVersionFixtures(path string) (*githubVersionsFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var fixture githubVersionsFixture
	if err := yaml.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("failed to parse githubrelease fixture: %w", err)
	}

	return &fixture, nil
}

// discoverTestCases finds all subdirectories in testdata directory (1 level deep).
func discoverTestCases(testdataDir string) ([]string, error) {
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		return nil, err
	}

	var testCases []string
	for _, entry := range entries {
		if entry.IsDir() {
			testCases = append(testCases, entry.Name())
		}
	}

	return testCases, nil
}

// runTestCase runs a single test case by:
// 1. Loading tring.yaml to get all group names
// 2. Discovering input/expected files based on naming convention
// 3. Running apply for each group and comparing results
func runTestCase(t *testing.T, caseName string, substitutions map[string]string) {
	testDir := filepath.Join("testdata", caseName)
	tempDir := t.TempDir()

	// Load tring.yaml to get group names
	cfg, err := loadTestConfig(filepath.Join(testDir, "tring.yaml"))
	if err != nil {
		t.Fatalf("failed to load tring.yaml: %v", err)
	}

	// Copy tring.yaml with URL substitution
	if err := copyFileWithSubstitutions(
		filepath.Join(testDir, "tring.yaml"),
		filepath.Join(tempDir, "tring.yaml"),
		substitutions,
	); err != nil {
		t.Fatalf("failed to copy tring.yaml: %v", err)
	}

	// Process each group
	for _, group := range cfg.Groups {
		groupName := group.Name

		// Discover input files for this group
		filePairs, err := discoverFilePairs(testDir, groupName)
		if err != nil {
			t.Fatalf("failed to discover file pairs for group %s: %v", groupName, err)
		}

		if len(filePairs) == 0 {
			t.Fatalf("no input files found for group %s", groupName)
		}

		// Copy input files to temp directory
		for _, pair := range filePairs {
			if err := copyFileWithSubstitutions(
				filepath.Join(testDir, pair.input),
				filepath.Join(tempDir, pair.targetName),
				substitutions,
			); err != nil {
				t.Fatalf("failed to copy input file %s: %v", pair.input, err)
			}
		}

		// Log registered types for debugging
		t.Logf("Registered source types: %v", sources.RegisteredTypes())
		t.Logf("Running group: %s", groupName)

		// Run apply
		var buf bytes.Buffer
		err = apply.Run(context.Background(), apply.Options{
			ConfigPath:   filepath.Join(tempDir, "tring.yaml"),
			GroupName:    groupName,
			DryRun:       false,
			ShowDiffLink: false,
			Output:       &buf,
		})
		if err != nil {
			t.Fatalf("apply failed for group %s: %v", groupName, err)
		}

		// Compare results with expected for each file
		for _, pair := range filePairs {
			actualPath := filepath.Join(tempDir, pair.targetName)
			expectedPath := filepath.Join(testDir, pair.expected)

			actual, err := os.ReadFile(actualPath)
			if err != nil {
				t.Fatalf("failed to read actual file %s: %v", pair.targetName, err)
			}

			expected, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("failed to read expected file %s: %v", pair.expected, err)
			}

			if string(actual) != string(expected) {
				t.Errorf("output mismatch for %s (group: %s)\n=== actual ===\n%s\n=== expected ===\n%s",
					pair.targetName, groupName, actual, expected)
			}
		}
	}
}

// filePair represents input file, expected file, and the target name used at runtime
type filePair struct {
	input      string // e.g., "test-group.input.go.mod"
	expected   string // e.g., "test-group.expected.go.mod"
	targetName string // e.g., "go.mod"
}

// discoverFilePairs finds input/expected file pairs for a group using naming convention:
// <group name>.input.<file name> and <group name>.expected.<file name>
func discoverFilePairs(testDir, groupName string) ([]filePair, error) {
	entries, err := os.ReadDir(testDir)
	if err != nil {
		return nil, err
	}

	prefix := groupName + ".input."
	var pairs []filePair

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		// Extract the file name part after "<group>.input."
		fileName := strings.TrimPrefix(name, prefix)
		expectedName := groupName + ".expected." + fileName

		// Verify expected file exists
		expectedPath := filepath.Join(testDir, expectedName)
		if _, err := os.Stat(expectedPath); err != nil {
			return nil, fmt.Errorf("expected file %q not found for input %q: %w", expectedName, name, err)
		}

		pairs = append(pairs, filePair{
			input:      name,
			expected:   expectedName,
			targetName: fileName,
		})
	}

	return pairs, nil
}

// loadTestConfig loads a tring.yaml file and returns the config
func loadTestConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return config.Parse(data)
}

// copyFileWithSubstitutions copies a file and replaces all placeholders with their values
func copyFileWithSubstitutions(src, dst string, substitutions map[string]string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	content := string(data)
	for placeholder, value := range substitutions {
		content = strings.ReplaceAll(content, placeholder, value)
	}

	return os.WriteFile(dst, []byte(content), 0644)
}

// setupTestModules adds test modules to the proxy server.
func setupTestModules(proxy *testproxy.Server, fixtures *versionsFixture) error {
	for modulePath, entries := range fixtures.ProxyModules {
		versions := make([]testproxy.ModuleVersion, 0, len(entries))
		for _, entry := range entries {
			releasedAt, err := time.Parse(time.RFC3339, entry.ReleasedAt)
			if err != nil {
				return fmt.Errorf("invalid released_at for module %q version %q: %w", modulePath, entry.Version, err)
			}
			versions = append(versions, testproxy.ModuleVersion{
				Version:    entry.Version,
				ReleasedAt: releasedAt,
			})
		}
		proxy.AddModule(modulePath, versions)
	}

	return nil
}

// setupTestRepos adds test repositories to the GitHub API server.
func setupTestRepos(github *testgithub.Server, fixtures *versionsFixture) error {
	for repoKey, entries := range fixtures.GitHubRepos {
		parts := strings.SplitN(repoKey, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid github repo key: %q (expected owner/repo)", repoKey)
		}

		tags := make([]testgithub.TagVersion, 0, len(entries))
		for _, entry := range entries {
			commitDate, err := time.Parse(time.RFC3339, entry.CommitDate)
			if err != nil {
				return fmt.Errorf("invalid commit_date for repo %q tag %q: %w", repoKey, entry.Name, err)
			}
			tags = append(tags, testgithub.TagVersion{
				Name:       entry.Name,
				CommitSHA:  entry.CommitSHA,
				CommitDate: commitDate,
			})
		}

		github.AddRepo(parts[0], parts[1], tags)
	}

	return nil
}
