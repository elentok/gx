package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMigrateFixtureEpic(t *testing.T, root, epicName string, files map[string]string) string {
	t.Helper()
	issuesDir := filepath.Join(root, epicName, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("mkdir issues: %v", err)
	}
	for name, content := range files {
		writeTicketFile(t, filepath.Join(issuesDir, name), content)
	}
	return issuesDir
}

func TestExecute_TicketsMigrate_Success(t *testing.T) {
	root := t.TempDir()
	issuesDir := writeMigrateFixtureEpic(t, root, "widget-epic", map[string]string{
		// Malformed fork chain: 01 lists both its direct fork (01a) and its
		// grandchild (01b, actually 01a's child) as children.
		"01-root.md":         "---\nid: \"01\"\nstatus: open\ntype: task\nchildren: [\"01a\", \"01b\"]\n---\nBody.\n",
		"01a-fork.md":        "---\nid: \"01a\"\nstatus: open\ntype: task\nparent: \"01\"\n---\nBody.\n",
		"01b-grandchild.md":  "---\nid: \"01b\"\nstatus: open\ntype: task\nparent: \"01a\"\n---\nBody.\n",
		"02-handed-back.md":  "---\nid: \"02\"\nstatus: ready-for-human\ntype: task\n---\nBody.\n",
		"03-no-status.md":    "---\nid: \"03\"\ntype: task\n---\nBody.\n",
		"04-already-new.md":  "---\nid: \"04\"\nstatus: draft\ntype: task\n---\nBody.\n",
		"05-self-blocked.md": "---\nid: \"05\"\nstatus: open\ntype: task\nparent: \"01\"\nblocked_by: [\"01\", \"02\"]\n---\nBody.\n",
		// A children value that never parsed as a list of IDs is still the
		// retired shape, and still has to be stripped and reported.
		"06-scalar-children.md": "---\nid: \"06\"\nstatus: open\ntype: task\nchildren: 06a\n---\nBody.\n",
	})
	alreadyNewPath := filepath.Join(issuesDir, "04-already-new.md")
	alreadyNewBefore, err := os.ReadFile(alreadyNewPath)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: bytes.NewBuffer(nil)}

	if err := execute([]string{"tickets", "migrate", root}, d); err != nil {
		t.Fatalf("execute tickets migrate: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"01-root.md: children removed",
		"02-handed-back.md: status: ready-for-human -> needs-info",
		"03-no-status.md: status: (missing) -> open",
		`05-self-blocked.md: blocked_by: removed self-parent entry "01"`,
		"06-scalar-children.md: children removed",
		"5 file(s) changed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want it to contain %q", out, want)
		}
	}
	if strings.Contains(out, "04-already-new.md") {
		t.Errorf("stdout = %q, want no report line for the already-new-shape ticket", out)
	}

	alreadyNewAfter, err := os.ReadFile(alreadyNewPath)
	if err != nil {
		t.Fatalf("reading fixture after migrate: %v", err)
	}
	if string(alreadyNewAfter) != string(alreadyNewBefore) {
		t.Errorf("already-new-shape ticket file changed: got %q, want unchanged %q", alreadyNewAfter, alreadyNewBefore)
	}

	rootRaw, err := os.ReadFile(filepath.Join(issuesDir, "01-root.md"))
	if err != nil {
		t.Fatalf("reading migrated root ticket: %v", err)
	}
	if strings.Contains(string(rootRaw), "children:") {
		t.Errorf("01-root.md = %q, want children field removed", string(rootRaw))
	}

	blockedRaw, err := os.ReadFile(filepath.Join(issuesDir, "05-self-blocked.md"))
	if err != nil {
		t.Fatalf("reading migrated self-blocked ticket: %v", err)
	}
	if !strings.Contains(string(blockedRaw), `"02"`) {
		t.Errorf("05-self-blocked.md = %q, want the non-self blocked_by entry (02) preserved", string(blockedRaw))
	}
	if strings.Contains(string(blockedRaw), `"01"`) && !strings.Contains(string(blockedRaw), `parent: "01"`) {
		t.Errorf("05-self-blocked.md = %q, want self-parent blocked_by entry (01) dropped", string(blockedRaw))
	}

	// Every migrated ticket now validates, and the epic loads with no
	// dangling/cyclic parent edges.
	for name := range map[string]string{
		"01-root.md": "", "01a-fork.md": "", "01b-grandchild.md": "", "02-handed-back.md": "",
		"03-no-status.md": "", "04-already-new.md": "", "05-self-blocked.md": "",
	} {
		var validateOut bytes.Buffer
		vd := deps{stdout: &validateOut, stderr: bytes.NewBuffer(nil)}
		if err := execute([]string{"tickets", "validate", filepath.Join(issuesDir, name)}, vd); err != nil {
			t.Errorf("validating migrated %s: %v", name, err)
		}
	}

	// Second run is a no-op: no changes reported, bytes untouched.
	beforeSecondRun := map[string][]byte{}
	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		t.Fatalf("reading issues dir: %v", err)
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(issuesDir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s before second run: %v", e.Name(), err)
		}
		beforeSecondRun[e.Name()] = raw
	}

	var secondStdout bytes.Buffer
	d2 := deps{stdout: &secondStdout, stderr: bytes.NewBuffer(nil)}
	if err := execute([]string{"tickets", "migrate", root}, d2); err != nil {
		t.Fatalf("second execute tickets migrate: %v", err)
	}
	if strings.TrimSpace(secondStdout.String()) != "no changes" {
		t.Errorf("second run stdout = %q, want %q", secondStdout.String(), "no changes")
	}
	for name, before := range beforeSecondRun {
		after, err := os.ReadFile(filepath.Join(issuesDir, name))
		if err != nil {
			t.Fatalf("reading %s after second run: %v", name, err)
		}
		if string(after) != string(before) {
			t.Errorf("%s changed on second (idempotent) run: got %q, want unchanged %q", name, after, before)
		}
	}
}

func TestExecute_TicketsMigrate_InvalidResultWritesNothing(t *testing.T) {
	root := t.TempDir()
	issuesDir := writeMigrateFixtureEpic(t, root, "cyclic-epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\nparent: \"02\"\n---\nBody.\n",
		"02-b.md": "---\nid: \"02\"\nstatus: open\ntype: task\nparent: \"01\"\n---\nBody.\n",
	})

	before := map[string][]byte{}
	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		t.Fatalf("reading issues dir: %v", err)
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(issuesDir, e.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", e.Name(), err)
		}
		before[e.Name()] = raw
	}

	var stdout, stderr bytes.Buffer
	d := deps{stdout: &stdout, stderr: &stderr}

	err = execute([]string{"tickets", "migrate", root}, d)
	if err == nil {
		t.Fatal("expected an error for a cyclic parent graph, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want it to name the cyclic parent graph", err.Error())
	}

	for name, want := range before {
		got, err := os.ReadFile(filepath.Join(issuesDir, name))
		if err != nil {
			t.Fatalf("reading %s after failed migrate: %v", name, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s changed despite failed migration: got %q, want unchanged %q", name, got, want)
		}
	}
}
