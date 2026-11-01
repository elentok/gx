package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupPublish is project "mine" with a manifest of three tickets (b waits on a,
// c on b) and their body files in a temp dir.
func setupPublish(t *testing.T) (d deps, project, manifest string) {
	t.Helper()
	cwd, project := setupEpicAddProject(t)
	d = deps{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, getwd: func() (string, error) { return cwd, nil }, env: envFor(t)}
	dir := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		writeTicketFile(t, filepath.Join(dir, n+".md"), "# "+n+"\n\nsee {{a}}\n")
	}
	manifest = writeManifest(t, dir, `- slug: a
  type: implement
  expected_context_window: 40000
  body: a.md
- slug: b
  type: implement
  blocked_by: [a]
  body: b.md
- slug: c
  type: implement
  blocked_by: [b]
  body: c.md
`)
	return d, project, manifest
}

func writeManifest(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func publishOut(d deps) string { return d.stdout.(*bytes.Buffer).String() }

func issueNames(t *testing.T, project, epic string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(project, epic, "issues"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestPublishCreatesDraftLoopEpicAndOpensTickets(t *testing.T) {
	d, project, manifest := setupPublish(t)

	if err := execute([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err != nil {
		t.Fatalf("publish: %v", err)
	}
	out := publishOut(d)
	for _, want := range []string{"a  ", "mine:e/01", "mine:e/02", "mine:e/03", "40000"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	md, _ := os.ReadFile(filepath.Join(project, "e", "ticket.md"))
	if !strings.Contains(string(md), "kind: loop") || !strings.Contains(string(md), "status: draft") {
		t.Errorf("epic ticket.md:\n%s", md)
	}
	raw, err := os.ReadFile(filepath.Join(project, "e", "issues", "02-b.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "status: open") || !strings.Contains(string(raw), "see 01") {
		t.Errorf("ticket 02:\n%s", raw)
	}
}

func TestPublishJSONAndDryRun(t *testing.T) {
	d, project, manifest := setupPublish(t)

	if err := execute([]string{"tickets", "publish", "mine:e", "--manifest", manifest, "--dry-run", "--json"}, d); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	var rows []publishedTicket
	if err := json.Unmarshal([]byte(publishOut(d)), &rows); err != nil {
		t.Fatalf("json: %v\n%s", err, publishOut(d))
	}
	if len(rows) != 3 || rows[1].Slug != "b" || rows[1].Address != "mine:e/02" || rows[0].Window != 40000 {
		t.Errorf("rows = %+v", rows)
	}
	if _, err := os.Stat(filepath.Join(project, "e")); !os.IsNotExist(err) {
		t.Errorf("dry-run created the epic: %v", err)
	}
}

func TestPublishIntoExistingEpicKeepsItsTickets(t *testing.T) {
	d, project, manifest := setupPublish(t)
	writeEpicTicket(t, project, "e", "01-old.md", ticketFrontmatter("01", ""))
	if err := os.WriteFile(filepath.Join(project, "e", "ticket.md"), []byte("---\nkind: loop\nstatus: open\n---\n\n# e\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := execute([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err != nil {
		t.Fatalf("publish: %v", err)
	}
	got := strings.Join(issueNames(t, project, "e"), " ")
	if got != "01-old.md 02-a.md 03-b.md 04-c.md" {
		t.Errorf("issues = %s", got)
	}
}

func TestPublishRollback(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new epic", true: "existing epic"}[existing], func(t *testing.T) {
			d, project, manifest := setupPublish(t)
			if existing {
				writeEpicTicket(t, project, "e", "01-old.md", ticketFrontmatter("01", ""))
				if err := os.WriteFile(filepath.Join(project, "e", "ticket.md"), []byte("---\nkind: loop\nstatus: open\n---\n\n# e\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			wantFiles := len(issueNames(t, project, "e"))
			writes := 0
			d.writeFile = func(path string, data []byte, perm os.FileMode) error {
				writes++
				if writes == 3 { // the last draft
					return errors.New("disk full")
				}
				return os.WriteFile(path, data, perm)
			}
			if err := execute([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err == nil {
				t.Fatalf("existing=%v: publish succeeded despite the fault", existing)
			}
			if got := len(issueNames(t, project, "e")); got != wantFiles {
				t.Errorf("existing=%v: %d issue files after rollback, want %d", existing, got, wantFiles)
			}
			if _, err := os.Stat(filepath.Join(project, "e")); existing == os.IsNotExist(err) {
				t.Errorf("existing=%v: epic dir presence wrong after rollback (err=%v)", existing, err)
			}

			d.writeFile = nil
			d.stdout = &bytes.Buffer{}
			if err := execute([]string{"tickets", "publish", "mine:e", "--manifest", manifest, "--json"}, d); err != nil {
				t.Fatalf("existing=%v: retry: %v", existing, err)
			}
			var rows []publishedTicket
			if err := json.Unmarshal([]byte(publishOut(d)), &rows); err != nil {
				t.Fatal(err)
			}
			first := "mine:e/01"
			if existing {
				first = "mine:e/02"
			}
			if rows[0].Address != first {
				t.Errorf("existing=%v: retry first address = %s, want %s", existing, rows[0].Address, first)
			}
		})
	}
}

func TestPublishRollbackWhenOpeningFails(t *testing.T) {
	d, project, manifest := setupPublish(t)
	writes := 0
	d.writeFile = func(path string, data []byte, perm os.FileMode) error {
		writes++
		if writes == 6 { // the last open
			return errors.New("disk full")
		}
		return os.WriteFile(path, data, perm)
	}
	if err := execute([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err == nil {
		t.Fatal("publish succeeded despite the fault")
	}
	if _, err := os.Stat(filepath.Join(project, "e")); !os.IsNotExist(err) {
		t.Errorf("epic left behind: %v", err)
	}
}

func TestPublishOpensOnce(t *testing.T) {
	d, _, manifest := setupPublish(t)
	var statuses []string
	d.writeFile = func(path string, data []byte, perm os.FileMode) error {
		status := "draft"
		if strings.Contains(string(data), "status: open") {
			status = "open"
		}
		statuses = append(statuses, status)
		return os.WriteFile(path, data, perm)
	}
	if err := execute([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got := strings.Join(statuses, " "); got != "draft draft draft open open open" {
		t.Errorf("write order = %s, want all drafts then all opens", got)
	}
}

func TestPublishInvalidTicketPrintsOnlyThatAndWritesNothing(t *testing.T) {
	d, project, manifest := setupPublish(t)
	raw, _ := os.ReadFile(manifest)
	if err := os.WriteFile(manifest, []byte(strings.Replace(string(raw), "type: implement\n  blocked_by: [a]", "type: task\n  blocked_by: [a]", 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	err := execute([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, d)
	if err == nil || err.Error() != `b: unknown type "task"` {
		t.Fatalf("err = %v, want only ticket b's type failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(project, "e")); !os.IsNotExist(statErr) {
		t.Errorf("epic left behind: %v", statErr)
	}
}

func TestCheckRenderedTicketsReportsOnlyTheInvalidOne(t *testing.T) {
	good := []byte("---\nid: \"01\"\nstatus: open\ntype: implement\n---\nBody.\n")
	bad := []byte("---\nid: \"02\"\nstatus: open\ntype: task\n---\nBody.\n")
	p := &publishPlan{Tickets: []plannedTicket{
		{Path: "/x/issues/01-a.md", Open: good},
		{Path: "/x/issues/02-b.md", Open: bad},
	}}
	errs := checkRenderedTickets(p)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "02-b.md") {
		t.Fatalf("errs = %v, want one failure naming 02-b.md", errs)
	}
}

func TestPublishManifestRefusalsWriteNothing(t *testing.T) {
	for name, content := range map[string]string{
		"cycle":         "- {slug: a, type: implement, blocked_by: [b], body: a.md}\n- {slug: b, type: implement, blocked_by: [a], body: b.md}\n",
		"unknown ref":   "- {slug: a, type: implement, body: x.md}\n",
		"duplicate":     "- {slug: a, type: implement, body: a.md}\n- {slug: a, type: implement, body: b.md}\n",
		"reserved":      "- {slug: commit, type: implement, body: a.md}\n",
		"digits only":   "- {slug: \"07\", type: implement, body: a.md}\n",
		"missing body":  "- {slug: a, type: implement, body: nope.md}\n",
		"unknown {{x}}": "- {slug: a, type: implement, body: bad.md}\n",
	} {
		t.Run(name, func(t *testing.T) {
			d, project, manifest := setupPublish(t)
			dir := filepath.Dir(manifest)
			writeTicketFile(t, filepath.Join(dir, "bad.md"), "see {{nope}}\n")
			writeManifest(t, dir, content)
			if err := execute([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err == nil {
				t.Fatal("publish succeeded")
			}
			if _, err := os.Stat(filepath.Join(project, "e")); !os.IsNotExist(err) {
				t.Errorf("epic left behind: %v", err)
			}
		})
	}
}

// publishLint publishes one ticket of type typ with the given body.
func publishLint(t *testing.T, typ, body string, extra ...string) (d deps, project string, err error) {
	t.Helper()
	d, project, manifest := setupPublish(t)
	dir := filepath.Dir(manifest)
	writeTicketFile(t, filepath.Join(dir, "a.md"), "# a\n\n"+body+"\n")
	writeManifest(t, dir, "- {slug: a, type: "+typ+", body: a.md}\n")
	err = execute(append([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, extra...), d)
	return d, project, err
}

func TestPublishClaimMissRefusesAndWritesNothing(t *testing.T) {
	d, project, err := publishLint(t, "implement", "see `docs/nowhere.md`")
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 1 {
		t.Fatalf("err = %v, want exit 1", err)
	}
	if out := publishOut(d); !strings.Contains(out, "miss: a: docs/nowhere.md") || strings.Contains(out, "SLUG") {
		t.Errorf("output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(project, "e")); !os.IsNotExist(err) {
		t.Errorf("epic left behind: %v", err)
	}
}

func TestPublishAllowClaimMisses(t *testing.T) {
	d, project, err := publishLint(t, "implement", "see `docs/nowhere.md`", "--allow-claim-misses")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	out := publishOut(d)
	if i, j := strings.Index(out, "miss: a:"), strings.Index(out, "SLUG"); i < 0 || j < i {
		t.Errorf("lint output must print before the table:\n%s", out)
	}
	if got := strings.Join(issueNames(t, project, "e"), " "); got != "01-a.md" {
		t.Errorf("issues = %s", got)
	}
}

func TestPublishDryRunPrintsLintAndRefuses(t *testing.T) {
	d, _, err := publishLint(t, "implement", "see `docs/nowhere.md`", "--dry-run")
	if err == nil || !strings.Contains(publishOut(d), "miss: a:") {
		t.Errorf("err = %v, out:\n%s", err, publishOut(d))
	}
}

func TestPublishVerifyMissIsAWarning(t *testing.T) {
	d, _, err := publishLint(t, "verify", "see `docs/nowhere.md`")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !strings.Contains(publishOut(d), "warning: a: docs/nowhere.md") {
		t.Errorf("output:\n%s", publishOut(d))
	}
}

func TestPublishPlannedNameDoesNotRefuse(t *testing.T) {
	d, project, manifest := setupPublish(t)
	dir := filepath.Dir(manifest)
	writeTicketFile(t, filepath.Join(dir, "a.md"), "# a\n\n## What to build\n\nAdd `docs/nowhere.md`.\n")
	writeTicketFile(t, filepath.Join(dir, "b.md"), "# b\n\n## Test seams\n\nuse `docs/nowhere.md`\n")
	writeManifest(t, dir, "- {slug: a, type: implement, body: a.md}\n- {slug: b, type: implement, blocked_by: [a], body: b.md}\n")
	if err := execute([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err != nil {
		t.Fatalf("publish: %v\n%s", err, publishOut(d))
	}
	if got := strings.Join(issueNames(t, project, "e"), " "); got != "01-a.md 02-b.md" {
		t.Errorf("issues = %s", got)
	}
}
