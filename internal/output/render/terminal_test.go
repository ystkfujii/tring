package render_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/domain/model"
	"github.com/ystkfujii/tring/internal/output/render"
)

func mustParse(s string) *semver.Version {
	v, err := semver.NewVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestTerminalRendererShowsErrorDetail(t *testing.T) {
	// Test that ErrorDetail is shown for resolve errors
	var buf bytes.Buffer
	renderer := render.NewTerminalRenderer(&buf, false)

	cs := model.ChangeSet{
		GroupName: "test-group",
		Changes: []model.PlannedChange{
			{
				Dependency: model.Dependency{
					Name:       "github.com/unknown/module",
					SourceKind: "gomod",
					FilePath:   "go.mod",
				},
				CurrentVersion: mustParse("v1.0.0"),
				SkipReason:     model.SkipReasonResolveError,
				ErrorDetail:    "failed to fetch version list for github.com/unknown/module: unexpected status code: 500",
			},
		},
	}

	renderer.Render(cs)

	output := buf.String()

	// Should contain the error detail
	if !strings.Contains(output, "unexpected status code: 500") {
		t.Errorf("Output should contain error detail:\n%s", output)
	}

	// Should show resolve error reason
	if !strings.Contains(output, "failed to resolve") {
		t.Errorf("Output should contain resolve error message:\n%s", output)
	}
}

func TestTerminalRendererSkippedWithoutErrorDetail(t *testing.T) {
	// Test that skipped items without ErrorDetail work correctly
	var buf bytes.Buffer
	renderer := render.NewTerminalRenderer(&buf, false)

	cs := model.ChangeSet{
		GroupName: "test-group",
		Changes: []model.PlannedChange{
			{
				Dependency: model.Dependency{
					Name:       "github.com/foo/bar",
					SourceKind: "gomod",
					FilePath:   "go.mod",
				},
				CurrentVersion: mustParse("v1.0.0"),
				SkipReason:     model.SkipReasonAlreadyLatest,
			},
		},
	}

	renderer.Render(cs)

	output := buf.String()

	// Should show already at latest version
	if !strings.Contains(output, "already at latest") {
		t.Errorf("Output should contain 'already at latest':\n%s", output)
	}

	// Should not have empty parentheses
	if strings.Contains(output, "()") {
		t.Errorf("Output should not contain empty parentheses:\n%s", output)
	}
}

func TestTerminalRendererEnvfileFormat(t *testing.T) {
	// Test that envfile dependencies show variable name
	var buf bytes.Buffer
	renderer := render.NewTerminalRenderer(&buf, false)

	cs := model.ChangeSet{
		GroupName: "test-group",
		Changes: []model.PlannedChange{
			{
				Dependency: model.Dependency{
					Name:       "k8s.io/api",
					SourceKind: "envfile",
					FilePath:   "versions.env",
					Locator:    "K8S_API_VERSION",
				},
				CurrentVersion: mustParse("v0.28.0"),
				SkipReason:     model.SkipReasonResolveError,
				ErrorDetail:    "module not found",
			},
		},
	}

	renderer.Render(cs)

	output := buf.String()

	// Should show variable name with module name
	if !strings.Contains(output, "K8S_API_VERSION") {
		t.Errorf("Output should contain variable name:\n%s", output)
	}
	if !strings.Contains(output, "k8s.io/api") {
		t.Errorf("Output should contain module name:\n%s", output)
	}
	if !strings.Contains(output, "module not found") {
		t.Errorf("Output should contain error detail:\n%s", output)
	}
}
