package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestExecute_TicketsValidate_RejectsPreContractionShape covers the
// lifecycle-contract contraction: the loader accepts only the post-migration
// frontmatter shape. Each case is a ticket file that `gx tickets migrate`
// would have already rewritten, so anything still carrying the old shape is a
// hand-authored file rather than a tracker the migration missed.
func TestExecute_TicketsValidate_RejectsPreContractionShape(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "children field",
			raw:  "---\nid: \"04\"\nstatus: open\ntype: task\nchildren:\n  - 04a\n---\nBody.\n",
			want: "children",
		},
		{
			name: "needs-triage status",
			raw:  "---\nid: \"04\"\nstatus: needs-triage\ntype: task\n---\nBody.\n",
			want: "needs-triage",
		},
		{
			name: "ready-for-agent status",
			raw:  "---\nid: \"04\"\nstatus: ready-for-agent\ntype: task\n---\nBody.\n",
			want: "ready-for-agent",
		},
		{
			name: "ready-for-human status",
			raw:  "---\nid: \"04\"\nstatus: ready-for-human\ntype: task\n---\nBody.\n",
			want: "ready-for-human",
		},
		{
			name: "needs-info status",
			raw:  "---\nid: \"04\"\nstatus: needs-info\ntype: task\n---\nBody.\n",
			want: "needs-answer",
		},
		{
			name: "needs-attention status",
			raw:  "---\nid: \"04\"\nstatus: needs-attention\ntype: task\n---\nBody.\n",
			want: "needs-repair",
		},
		{
			name: "missing status",
			raw:  "---\nid: \"04\"\ntype: task\n---\nBody.\n",
			want: "status",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "04-ticket.md")
			writeTicketFile(t, path, tc.raw)

			var stdout, stderr bytes.Buffer
			err := execute([]string{"tickets", "validate", path}, deps{stdout: &stdout, stderr: &stderr})
			if err == nil {
				t.Fatalf("execute tickets validate: want an error, got none (stdout %q)", stdout.String())
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestExecute_TicketsValidate_AcceptsPostMigrationShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "04a-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04a\"\nstatus: needs-answer\ntype: task\nparent: \"04\"\n---\nBody.\n")

	var stdout, stderr bytes.Buffer
	if err := execute([]string{"tickets", "validate", path}, deps{stdout: &stdout, stderr: &stderr}); err != nil {
		t.Fatalf("execute tickets validate: %v", err)
	}
}

// TestExecute_TicketsSet_NoChildrenFlag pins that `set` no longer offers a way
// to write the retired field back onto a ticket the loader would then reject.
func TestExecute_TicketsSet_NoChildrenFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "04b-ticket.md")
	writeTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: task\n---\nBody.\n")

	var stdout, stderr bytes.Buffer
	err := execute([]string{"tickets", "set", path, "--children=04c"}, deps{stdout: &stdout, stderr: &stderr})
	if err == nil {
		t.Fatal("execute tickets set --children: want an unknown-flag error, got none")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %q, want an unknown-flag error", err)
	}
}
