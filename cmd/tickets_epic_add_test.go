package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

func setupEpicAddProject(t *testing.T) (cwd, project string) {
	t.Helper()
	store := isolateTicketStore(t)
	repo := testutil.TempRepo(t)
	return repo, addProject(t, store, "mine", repo)
}

func TestRunTicketsEpicAdd_CreatesEpics(t *testing.T) {
	cwd, project := setupEpicAddProject(t)

	for _, tc := range []struct {
		arg, kind, status, body string
		want                    []string
	}{
		{"mine:m", "map", "draft", "", []string{"kind: map", "status: draft", "# m"}},
		{"mine:l", "loop", "open", "", []string{"kind: loop", "status: open"}},
		{"bare", "loop", "open", "# Title\n\nwords\n", []string{"kind: loop", "# Title", "words"}},
		{"mine:p", "loop", "draft", "---\ndestination_unconfirmed: true\n---\n\n# P\n", []string{"destination_unconfirmed: true", "# P"}},
	} {
		var out bytes.Buffer
		if err := runTicketsEpicAdd(cwd, tc.arg, tc.kind, tc.status, tc.body, &out); err != nil {
			t.Fatalf("%s: %v", tc.arg, err)
		}
		name := strings.TrimPrefix(tc.arg, "mine:")
		raw, err := os.ReadFile(filepath.Join(project, name, "ticket.md"))
		if err != nil {
			t.Fatalf("%s: %v", tc.arg, err)
		}
		for _, w := range tc.want {
			if !strings.Contains(string(raw), w) {
				t.Errorf("%s: ticket.md missing %q:\n%s", tc.arg, w, raw)
			}
		}
		if got, want := out.String(), "mine:"+name+"\n"; got != want {
			t.Errorf("%s: printed %q, want %q", tc.arg, got, want)
		}
	}
	if raw, _ := os.ReadFile(filepath.Join(project, "p", "ticket.md")); strings.Contains(string(raw), "---\n---\n") || strings.Count(string(raw), "---\n") != 2 {
		t.Errorf("body frontmatter was not merged into one block:\n%s", raw)
	}
}

func TestRunTicketsEpicAdd_StampsCreatedAtAndMapCompletedAt(t *testing.T) {
	cwd, project := setupEpicAddProject(t)
	for _, arg := range []string{"mine:m", "mine:other"} {
		kind := "map"
		if arg == "mine:other" {
			kind = "loop"
		}
		if err := runTicketsEpicAdd(cwd, arg, kind, "open", "", &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	if tickets.LoadEpicTicketMD(filepath.Join(project, "m")).CreatedAt.IsZero() {
		t.Error("new map has no created_at")
	}
	if !tickets.LoadEpicTicketMD(filepath.Join(project, "m")).CompletedAt.IsZero() {
		t.Error("map completed before its impl epic exists")
	}
	if err := runTicketsEpicAdd(cwd, "mine:m-impl", "loop", "open", "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	m := tickets.LoadEpicTicketMD(filepath.Join(project, "m"))
	impl := tickets.LoadEpicTicketMD(filepath.Join(project, "m-impl"))
	if impl.CreatedAt.IsZero() || m.CompletedAt.IsZero() || !m.CompletedAt.Equal(impl.CreatedAt) {
		t.Errorf("map completed_at %v, impl created_at %v; want equal and set", m.CompletedAt, impl.CreatedAt)
	}
	if err := runTicketsEpicAdd(cwd, "mine:other-impl", "loop", "open", "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !tickets.LoadEpicTicketMD(filepath.Join(project, "other")).CompletedAt.IsZero() {
		t.Error("a loop epic's completed_at was stamped by its -impl sibling")
	}
}

func TestRunTicketsEpicAdd_RefusalsWriteNothing(t *testing.T) {
	cwd, project := setupEpicAddProject(t)
	if err := runTicketsEpicAdd(cwd, "mine:exists", "loop", "open", "", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(project, "exists", "ticket.md"))

	for _, tc := range []struct {
		name, arg, kind, body, wantErr string
	}{
		{"server-stamped", "mine:x", "loop", "---\ncompleted_at: 2026-01-01T00:00:00Z\n---\n\n# X\n", "server-stamped"},
		{"created_at", "mine:x", "loop", "---\ncreated_at: 2026-01-01T00:00:00Z\n---\n\n# X\n", "server-stamped"},
		{"merged_at", "mine:x", "loop", "---\nmerged_at: 2026-01-01T00:00:00Z\n---\n\n# X\n", "server-stamped"},
		{"merged_at cleared", "mine:x", "loop", "---\nmerged_at:\n---\n\n# X\n", "server-stamped"},
		{"unknown field", "mine:x", "loop", "---\nbogus: 1\n---\n\n# X\n", "unknown field"},
		{"existing", "mine:exists", "map", "", "already exists"},
		{"bad kind", "mine:x", "tree", "", "kind"},
		{"kind conflict", "mine:x", "map", "---\nkind: loop\n---\n\n# X\n", "conflicts"},
	} {
		err := runTicketsEpicAdd(cwd, tc.arg, tc.kind, "", tc.body, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.wantErr)
		}
		if _, statErr := os.Stat(filepath.Join(project, "x")); statErr == nil {
			t.Errorf("%s: refusal wrote an epic directory", tc.name)
		}
	}
	if after, _ := os.ReadFile(filepath.Join(project, "exists", "ticket.md")); string(after) != string(before) {
		t.Errorf("existing epic changed:\n%s", after)
	}

	err := runTicketsEpicAdd(cwd, "nope:x", "loop", "open", "", &bytes.Buffer{})
	var addrErr *tickets.AddressError
	if !errors.As(err, &addrErr) || addrErr.Code != tickets.CodeUnknownProject {
		t.Errorf("unknown project: err = %v, want %s", err, tickets.CodeUnknownProject)
	}
}

func TestExecute_TicketsEpicAdd(t *testing.T) {
	cwd, project := setupEpicAddProject(t)
	out, err := runIn(t, cwd, "tickets", "epic", "add", "mine:viaexec", "--kind", "map")
	if err != nil {
		t.Fatal(err)
	}
	if out != "mine:viaexec\n" {
		t.Errorf("printed %q", out)
	}
	if _, err := os.Stat(filepath.Join(project, "viaexec", "ticket.md")); err != nil {
		t.Error(err)
	}
}
