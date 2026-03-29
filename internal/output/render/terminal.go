package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/ystkfujii/tring/internal/domain/model"
)

// TerminalRenderer renders change sets to a terminal.
type TerminalRenderer struct {
	w        io.Writer
	showDiff bool
}

// NewTerminalRenderer creates a new terminal renderer.
func NewTerminalRenderer(w io.Writer, showDiff bool) *TerminalRenderer {
	return &TerminalRenderer{
		w:        w,
		showDiff: showDiff,
	}
}

// Render renders a change set to the terminal.
func (r *TerminalRenderer) Render(cs model.ChangeSet) {
	fmt.Fprintf(r.w, "Group: %s\n", cs.GroupName)
	fmt.Fprintf(r.w, "%s\n\n", strings.Repeat("=", len("Group: "+cs.GroupName)))

	updates := cs.Updates()
	skipped := cs.Skipped()

	if len(updates) == 0 && len(skipped) == 0 {
		fmt.Fprintln(r.w, "No dependencies found.")
		return
	}

	// Group updates by file
	if len(updates) > 0 {
		fmt.Fprintln(r.w, "Planned Changes:")

		updatesByFile := make(map[string][]model.PlannedChange)
		var fileOrder []string
		for _, u := range updates {
			if _, seen := updatesByFile[u.Dependency.FilePath]; !seen {
				fileOrder = append(fileOrder, u.Dependency.FilePath)
			}
			updatesByFile[u.Dependency.FilePath] = append(updatesByFile[u.Dependency.FilePath], u)
		}

		for _, file := range fileOrder {
			fmt.Fprintf(r.w, "  File: %s\n", file)
			for _, u := range updatesByFile[file] {
				r.renderUpdate(u)
			}
			fmt.Fprintln(r.w)
		}
	}

	// Render skipped dependencies
	if len(skipped) > 0 {
		fmt.Fprintln(r.w, "Skipped:")
		for _, s := range skipped {
			r.renderSkipped(s)
		}
		fmt.Fprintln(r.w)
	}

	// Summary
	updateCount, skipCount := cs.Stats()
	fmt.Fprintf(r.w, "Total: %d change(s), %d skipped\n", updateCount, skipCount)
}

func (r *TerminalRenderer) renderUpdate(c model.PlannedChange) {
	name := c.Dependency.Name

	// For envfile, show variable name too
	if c.Dependency.SourceKind == "envfile" {
		varName := c.Dependency.Locator
		name = fmt.Sprintf("%s (%s)", varName, c.Dependency.Name)
	}

	currentVer := "unknown"
	if c.CurrentVersion != nil {
		currentVer = c.CurrentVersion.Original()
	}

	targetVer := "unknown"
	if c.TargetVersion != nil {
		targetVer = c.TargetVersion.Original()
	}

	fmt.Fprintf(r.w, "    %s\n", name)
	fmt.Fprintf(r.w, "      %s -> %s (%s)\n", currentVer, targetVer, c.Strategy)

	if r.showDiff && c.DiffLink != "" {
		fmt.Fprintf(r.w, "      Diff: %s\n", c.DiffLink)
	}
}

func (r *TerminalRenderer) renderSkipped(c model.PlannedChange) {
	name := c.Dependency.Name

	// For envfile, show variable name too
	if c.Dependency.SourceKind == "envfile" {
		varName := c.Dependency.Locator
		name = fmt.Sprintf("%s (%s)", varName, c.Dependency.Name)
	}

	reason := c.SkipReason.String()
	if c.ErrorDetail != "" {
		reason = fmt.Sprintf("%s (%s)", reason, c.ErrorDetail)
	}

	fmt.Fprintf(r.w, "  %s: %s\n", name, reason)
}

// RenderDryRunHeader prints a header indicating dry-run mode.
func (r *TerminalRenderer) RenderDryRunHeader() {
	fmt.Fprintln(r.w, "[DRY RUN] No files will be modified.")
	fmt.Fprintln(r.w)
}

// RenderApplyHeader prints a header indicating apply mode.
func (r *TerminalRenderer) RenderApplyHeader() {
	fmt.Fprintln(r.w, "Applying changes...")
	fmt.Fprintln(r.w)
}

// RenderApplySuccess prints a success message after applying changes.
func (r *TerminalRenderer) RenderApplySuccess(updateCount int) {
	fmt.Fprintln(r.w)
	fmt.Fprintf(r.w, "Successfully applied %d change(s).\n", updateCount)
}

// RenderError prints an error message.
func (r *TerminalRenderer) RenderError(err error) {
	fmt.Fprintf(r.w, "Error: %v\n", err)
}
