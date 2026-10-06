package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validateStderr(t *testing.T, path string) (string, error) {
	t.Helper()
	stderr := bytes.NewBuffer(nil)
	d := deps{stdout: bytes.NewBuffer(nil), stderr: stderr}
	err := execute([]string{"tickets", "validate", path}, d)
	return stderr.String(), err
}

func TestExecute_TicketsValidate_BaseOnCommitlessIsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeEpicTicket(t, dir, "e", "01-a.md", ticketFrontmatter("01", "commitless: true\nbase: main\n"))

	_, err := validateStderr(t, path)
	if err == nil || !strings.Contains(err.Error(), "base: not allowed on a commitless ticket") {
		t.Errorf("error = %v, want base-on-commitless error", err)
	}
}

func TestExecute_TicketsValidate_BaseInVCSNoneProjectIsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{"vcs":"none"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	path := writeEpicTicket(t, dir, "e", "01-a.md", ticketFrontmatter("01", "base: main\n"))

	_, err := validateStderr(t, path)
	if err == nil || !strings.Contains(err.Error(), "base: not allowed in a vcs: none project") {
		t.Errorf("error = %v, want base-in-vcs-none error", err)
	}
}

func TestExecute_TicketsValidate_BaseOnCommitfulPasses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeEpicTicket(t, dir, "e", "01-a.md", ticketFrontmatter("01", "base: main\nresolved_base: main@abc123\n"))

	if stderr, err := validateStderr(t, path); err != nil || stderr != "" {
		t.Errorf("validate = %v, stderr %q, want clean pass", err, stderr)
	}
}

func TestExecute_TicketsValidate_BaseWarnings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		files map[string]string // "epic/file" -> frontmatter (id derived from file prefix)
		path  string
		want  string // empty: no warning
	}{
		{
			name: "two unlanded commitful blockers and no base",
			files: map[string]string{
				"e/01-a.md": "", "e/02-b.md": "",
				"e/03-c.md": "blocked_by:\n  - \"01\"\n  - \"02\"\n",
			},
			path: "e/03-c.md",
			want: "ticket 03: 2 unlanded commitful blockers and no base:",
		},
		{
			name: "explicit base silences the ambiguity",
			files: map[string]string{
				"e/01-a.md": "", "e/02-b.md": "",
				"e/03-c.md": "base: main\nblocked_by:\n  - \"01\"\n  - \"02\"\n",
			},
			path: "e/03-c.md",
		},
		{
			name: "commitless blockers don't count",
			files: map[string]string{
				"e/01-a.md": "commitless: true\n", "e/02-b.md": "",
				"e/03-c.md": "blocked_by:\n  - \"01\"\n  - \"02\"\n",
			},
			path: "e/03-c.md",
		},
		{
			name: "cross-epic commitful blocker",
			files: map[string]string{
				"other/01-a.md": "",
				"e/02-b.md":     "blocked_by:\n  - \"other/01\"\n",
			},
			path: "e/02-b.md",
			want: `ticket 02: blocker "other/01" is in another epic, so no base is derived`,
		},
		{
			name: "cross-epic commitless blocker",
			files: map[string]string{
				"other/01-a.md": "commitless: true\n",
				"e/02-b.md":     "blocked_by:\n  - \"other/01\"\n",
			},
			path: "e/02-b.md",
		},
		{
			name: "resolved_base differs from the last claim",
			files: map[string]string{
				"e/01-a.md": "resolved_base: main@bbb\n",
			},
			path: "e/01-a.md",
			want: "ticket 01: resolved_base main@bbb differs from the last claim (main@aaa)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			paths := map[string]string{}
			for rel, extra := range tt.files {
				epic, file := filepath.Split(rel)
				paths[rel] = writeEpicTicket(t, dir, strings.TrimSuffix(epic, "/"), file, ticketFrontmatter(file[:2], extra))
			}
			if strings.Contains(tt.name, "last claim") {
				log := `{"type":"iteration-started","ticket":"01","resolved_base":"main@aaa"}` + "\n"
				if err := os.WriteFile(filepath.Join(dir, "e", "run-log.jsonl"), []byte(log), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			stderr, err := validateStderr(t, paths[tt.path])
			if err != nil {
				t.Fatalf("warnings must not fail validate: %v", err)
			}
			if tt.want == "" {
				if stderr != "" {
					t.Errorf("stderr = %q, want no warning", stderr)
				}
				return
			}
			if !strings.Contains(stderr, "warning: ") || !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want warning containing %q", stderr, tt.want)
			}
		})
	}
}
