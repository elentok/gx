package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
)

// TestExecute_TicketsValidate_RejectsPreContractionShape covers the
// lifecycle-contract contraction: the loader accepts only the post-migration
// frontmatter shape. Each case is a ticket file that `gx tickets migrate`
// would have already rewritten, so anything still carrying the old shape is a
// hand-authored file rather than a tracker the migration missed.
func TestExecute_TicketsValidate_RejectsPreContractionShape(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "children field",
			raw:  "---\nid: \"04\"\nstatus: open\ntype: implement\nchildren:\n  - 04a\n---\nBody.\n",
			want: "children",
		},
		{
			name: "needs-triage status",
			raw:  "---\nid: \"04\"\nstatus: needs-triage\ntype: implement\n---\nBody.\n",
			want: "needs-triage",
		},
		{
			name: "ready-for-agent status",
			raw:  "---\nid: \"04\"\nstatus: ready-for-agent\ntype: implement\n---\nBody.\n",
			want: "ready-for-agent",
		},
		{
			name: "ready-for-human status",
			raw:  "---\nid: \"04\"\nstatus: ready-for-human\ntype: implement\n---\nBody.\n",
			want: "ready-for-human",
		},
		{
			name: "needs-info status",
			raw:  "---\nid: \"04\"\nstatus: needs-info\ntype: implement\n---\nBody.\n",
			want: "gx tickets migrate",
		},
		{
			name: "needs-attention status",
			raw:  "---\nid: \"04\"\nstatus: needs-attention\ntype: implement\n---\nBody.\n",
			want: "gx tickets migrate",
		},
		{
			name: "missing status",
			raw:  "---\nid: \"04\"\ntype: implement\n---\nBody.\n",
			want: "status",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "04-ticket.md")
			cmdtest.WriteTicketFile(t, path, tc.raw)

			var stdout, stderr bytes.Buffer
			err := execute([]string{"tickets", "validate", path}, Deps{Env: cmdbase.Env{Stdout: &stdout, Stderr: &stderr}})
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
	t.Parallel()
	path := filepath.Join(t.TempDir(), "04a-ticket.md")
	cmdtest.WriteTicketFile(t, path, "---\nid: \"04a\"\nstatus: paused\npause_reason: self-reported\ntype: implement\nparent: \"04\"\n---\nBody.\n")
	cmdtest.WriteLogFile(t, path, cmdtest.LogQuestionLine)

	var stdout, stderr bytes.Buffer
	if err := execute([]string{"tickets", "validate", path}, Deps{Env: cmdbase.Env{Stdout: &stdout, Stderr: &stderr}}); err != nil {
		t.Fatalf("execute tickets validate: %v", err)
	}
}

// TestExecute_TicketsSet_NoChildrenFlag pins that `set` no longer offers a way
// to write the retired field back onto a ticket the loader would then reject.
func TestExecute_TicketsSet_NoChildrenFlag(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "04b-ticket.md")
	cmdtest.WriteTicketFile(t, path, "---\nid: \"04b\"\nstatus: open\ntype: implement\n---\nBody.\n")

	var stdout, stderr bytes.Buffer
	err := execute([]string{"tickets", "set", path, "--children=04c"}, Deps{Env: cmdbase.Env{Stdout: &stdout, Stderr: &stderr}})
	if err == nil {
		t.Fatal("execute tickets set --children: want an unknown-flag error, got none")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %q, want an unknown-flag error", err)
	}
}
