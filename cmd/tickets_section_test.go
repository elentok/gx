package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets/schema"
)

func newAddEpic(t *testing.T) (epicPath, issuesDir string) {
	t.Helper()
	epicPath = filepath.Join(t.TempDir(), "widget-epic")
	issuesDir = filepath.Join(epicPath, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	testutil.EnsureEpicTicketMD(t, epicPath)
	return epicPath, issuesDir
}

func TestRunTicketsAddBody_CreatesOpenTicketUnderParent(t *testing.T) {
	t.Parallel()
	epicPath, issuesDir := newAddEpic(t)
	writeTicket(t, filepath.Join(issuesDir, "12-parent.md"), "12", "done", "implement")

	var stdout bytes.Buffer
	if err := runTicketsAddBody(epicPath, "12", "child", "## What to build\n\nDo it.\n", false, &stdout, io.Discard); err != nil {
		t.Fatalf("runTicketsAddBody: %v", err)
	}

	path := filepath.Join(issuesDir, "12a-child.md")
	ticket, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatalf("ticket failed validation: %v", err)
	}
	if ticket.Status != schema.StatusOpen {
		t.Errorf("status = %q, want open", ticket.Status)
	}
	if ticket.Parent == nil || *ticket.Parent != "12" {
		t.Errorf("parent = %v, want 12", ticket.Parent)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "Do it.") {
		t.Errorf("body missing from file:\n%s", raw)
	}
	if got := strings.TrimSpace(stdout.String()); !strings.HasSuffix(got, "widget-epic/12a") {
		t.Errorf("stdout = %q, want an address ending in widget-epic/12a", got)
	}
}

// The gx-to-tickets template opens with its own frontmatter: add merges it
// into the one block it writes instead of stacking a second one.
func TestRunTicketsAddBody_MergesBodyFrontmatter(t *testing.T) {
	t.Parallel()
	epicPath, issuesDir := newAddEpic(t)
	writeTicket(t, filepath.Join(issuesDir, "01-first.md"), "01", "open", "implement")
	body := "---\nid: \"07\"\nstatus: open\nblocked_by: [\"01\"]\ntype: grilling\nexpected_context_window: 60000\n---\n\n# Decide it\n\n## Question\n\nWhich?\n"

	var stdout, stderr bytes.Buffer
	if err := runTicketsAddBody(epicPath, "", "decide", body, false, &stdout, &stderr); err != nil {
		t.Fatalf("runTicketsAddBody: %v", err)
	}

	path := filepath.Join(issuesDir, "02-decide.md")
	ticket, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatalf("ticket failed validation: %v", err)
	}
	if ticket.Type != "grilling" || len(ticket.BlockedBy) != 1 || ticket.BlockedBy[0] != "01" || ticket.ExpectedContextWindow != 60000 {
		t.Errorf("frontmatter not merged: %+v", ticket)
	}
	raw, _ := os.ReadFile(path)
	if n := strings.Count(string(raw), "---\n"); n != 2 {
		t.Errorf("want one frontmatter block, got %d delimiters:\n%s", n, raw)
	}
	if !strings.Contains(string(raw), "## Question") {
		t.Errorf("body missing:\n%s", raw)
	}
	if !strings.Contains(stderr.String(), "allocated id wins") {
		t.Errorf("stderr = %q, want an id-mismatch warning", stderr.String())
	}
}

func TestRunTicketsAddBody_BadBodyFrontmatterRefusesWithoutWriting(t *testing.T) {
	t.Parallel()
	for name, fm := range map[string]string{
		"unknown field": "owner: me\n",
		"done status":   "status: done\n",
		"read-only":     "actual_context_window: 5\n",
		"bad type":      "type: chore\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			epicPath, issuesDir := newAddEpic(t)
			body := "---\n" + fm + "---\n\n# T\n\nBody.\n"
			if err := runTicketsAddBody(epicPath, "", "bad", body, false, io.Discard, io.Discard); err == nil {
				t.Fatal("want a refusal")
			}
			if entries, _ := os.ReadDir(issuesDir); len(entries) != 0 {
				t.Errorf("files written despite refusal: %v", entries)
			}
		})
	}
}

func TestRunTicketsAddBody_EmptyBodyFailsWithoutWriting(t *testing.T) {
	t.Parallel()
	epicPath, issuesDir := newAddEpic(t)

	var stdout bytes.Buffer
	if err := runTicketsAddBody(epicPath, "", "empty", "  \n", false, &stdout, io.Discard); err == nil {
		t.Fatal("want error for empty body, got nil")
	}
	entries, _ := os.ReadDir(issuesDir)
	if len(entries) != 0 {
		t.Fatalf("issues dir = %v, want empty", entries)
	}
}

func TestRunTicketsAddBody_JSON(t *testing.T) {
	t.Parallel()
	epicPath, _ := newAddEpic(t)

	var stdout bytes.Buffer
	if err := runTicketsAddBody(epicPath, "", "thing", "body\n", true, &stdout, io.Discard); err != nil {
		t.Fatalf("runTicketsAddBody: %v", err)
	}
	var got struct{ Address, Path string }
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout.String())
	}
	if !strings.HasSuffix(got.Address, "widget-epic/01") || !strings.HasSuffix(got.Path, "01-thing.md") {
		t.Errorf("got %+v", got)
	}
}

func TestSetSection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, body, heading, content, want string
	}{
		{
			name:    "replaces existing section, keeps neighbours",
			body:    "\n# T\n\n## A\n\nold\n\n## B\n\nkeep\n",
			heading: "A", content: "new\n",
			want: "\n# T\n\n## A\n\nnew\n\n## B\n\nkeep\n",
		},
		{
			name:    "replaces last section",
			body:    "\n# T\n\n## A\n\nold\n",
			heading: "A", content: "new",
			want: "\n# T\n\n## A\n\nnew\n",
		},
		{
			name:    "appends missing section",
			body:    "\n# T\n\n## A\n\nx\n",
			heading: "B", content: "new\n",
			want: "\n# T\n\n## A\n\nx\n\n## B\n\nnew\n",
		},
		{
			name:    "ignores heading-like lines inside code fences",
			body:    "\n## A\n\n```\n## B\n```\n\n## B\n\nold\n",
			heading: "B", content: "new\n",
			want: "\n## A\n\n```\n## B\n```\n\n## B\n\nnew\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := schema.SetSection(tt.body, tt.heading, tt.content); got != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestRunTicketsSection_WritesAndPrintsAddress(t *testing.T) {
	t.Parallel()
	_, issuesDir := newAddEpic(t)
	path := filepath.Join(issuesDir, "01-a.md")
	writeTicket(t, path, "01", "open", "implement")

	var stdout bytes.Buffer
	if err := runTicketsSection(path, "Notes", "hello\n", false, &stdout); err != nil {
		t.Fatalf("runTicketsSection: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "## Notes\n\nhello\n") {
		t.Errorf("section not written:\n%s", raw)
	}
	if _, err := schema.ParseTicket(path); err != nil {
		t.Errorf("ticket invalid after section write: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); !strings.HasSuffix(got, "widget-epic/01") {
		t.Errorf("stdout = %q, want an address", got)
	}
}
