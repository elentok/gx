package ticketsflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/tickets/ticketspec"
)

// shapeSections are the sections an implement ticket needs to pass the publish checks.
const shapeSections = "\n## Test seams\n\nnone — covered elsewhere\n\n## Where to look\n\nnowhere\n"

// setupPublish is project "mine" with a manifest of three tickets (b waits on a,
// c on b) and their body files in a temp dir.
func setupPublish(t *testing.T) (d Deps, project, manifest string) {
	t.Helper()
	cwd, project := setupEpicAddProject(t)
	d = Deps{Env: cmdbase.Env{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Getwd: func() (string, error) { return cwd, nil }, Env: envFor(t)}, CommandTree: fakeTree}
	dir := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		writeTicketFile(t, filepath.Join(dir, n+".md"), "# "+n+"\n\nsee {{a}}\n"+shapeSections)
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

func publishOut(d Deps) string { return d.Stdout.(*bytes.Buffer).String() }

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

	if err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err != nil {
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

	if err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest, "--dry-run", "--json"}, d); err != nil {
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

	if err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err != nil {
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
			d.WriteFile = func(path string, data []byte, perm os.FileMode) error {
				writes++
				if writes == 3 { // the last draft
					return errors.New("disk full")
				}
				return os.WriteFile(path, data, perm)
			}
			if err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err == nil {
				t.Fatalf("existing=%v: publish succeeded despite the fault", existing)
			}
			if got := len(issueNames(t, project, "e")); got != wantFiles {
				t.Errorf("existing=%v: %d issue files after rollback, want %d", existing, got, wantFiles)
			}
			if _, err := os.Stat(filepath.Join(project, "e")); existing == os.IsNotExist(err) {
				t.Errorf("existing=%v: epic dir presence wrong after rollback (err=%v)", existing, err)
			}

			d.WriteFile = nil
			d.Stdout = &bytes.Buffer{}
			if err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest, "--json"}, d); err != nil {
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
	d.WriteFile = func(path string, data []byte, perm os.FileMode) error {
		writes++
		if writes == 6 { // the last open
			return errors.New("disk full")
		}
		return os.WriteFile(path, data, perm)
	}
	if err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err == nil {
		t.Fatal("publish succeeded despite the fault")
	}
	if _, err := os.Stat(filepath.Join(project, "e")); !os.IsNotExist(err) {
		t.Errorf("epic left behind: %v", err)
	}
}

func TestPublishOpensOnce(t *testing.T) {
	d, _, manifest := setupPublish(t)
	var statuses []string
	d.WriteFile = func(path string, data []byte, perm os.FileMode) error {
		status := "draft"
		if strings.Contains(string(data), "status: open") {
			status = "open"
		}
		statuses = append(statuses, status)
		return os.WriteFile(path, data, perm)
	}
	if err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err != nil {
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

	err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest}, d)
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
			if err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err == nil {
				t.Fatal("publish succeeded")
			}
			if _, err := os.Stat(filepath.Join(project, "e")); !os.IsNotExist(err) {
				t.Errorf("epic left behind: %v", err)
			}
		})
	}
}

// publishLint publishes one ticket of type typ with the given body.
func publishLint(t *testing.T, typ, body string, extra ...string) (d Deps, project string, err error) {
	t.Helper()
	d, project, manifest := setupPublish(t)
	dir := filepath.Dir(manifest)
	writeTicketFile(t, filepath.Join(dir, "a.md"), "# a\n\n"+body+"\n"+shapeSections)
	writeManifest(t, dir, "- {slug: a, type: "+typ+", body: a.md}\n")
	err = execute(t, append([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, extra...), d)
	return d, project, err
}

func TestPublishClaimMissRefusesAndWritesNothing(t *testing.T) {
	d, project, err := publishLint(t, "implement", "see `docs/nowhere.md`")
	var ee *cmdbase.ExitError
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
	writeTicketFile(t, filepath.Join(dir, "a.md"), "# a\n\n## What to build\n\nAdd `docs/nowhere.md`.\n"+shapeSections)
	writeTicketFile(t, filepath.Join(dir, "b.md"), "# b\n\n## Test seams\n\nuse `docs/nowhere.md`\n\n## Where to look\n\nnowhere\n")
	writeManifest(t, dir, "- {slug: a, type: implement, body: a.md}\n- {slug: b, type: implement, blocked_by: [a], body: b.md}\n")
	if err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest}, d); err != nil {
		t.Fatalf("publish: %v\n%s", err, publishOut(d))
	}
	if got := strings.Join(issueNames(t, project, "e"), " "); got != "01-a.md 02-b.md" {
		t.Errorf("issues = %s", got)
	}
}

// publishShape publishes one ticket whose body is exactly body.
func publishShape(t *testing.T, typ string, window int, body string, extra ...string) (d Deps, project string, err error) {
	t.Helper()
	d, project, manifest := setupPublish(t)
	dir := filepath.Dir(manifest)
	writeTicketFile(t, filepath.Join(dir, "a.md"), "# a\n\n"+body)
	writeManifest(t, dir, fmt.Sprintf("- {slug: a, type: %s, expected_context_window: %d, body: a.md}\n", typ, window))
	err = execute(t, append([]string{"tickets", "publish", "mine:e", "--manifest", manifest}, extra...), d)
	return d, project, err
}

func TestPublishShapeChecks(t *testing.T) {
	const where = "\n## Where to look\n\nhere\n"
	cases := []struct {
		name    string
		typ     string
		window  int
		body    string
		wantErr string // empty: publish passes
	}{
		{"no test seams", "implement", 1000, where, "a: no ## Test seams"},
		{"empty test seams", "implement", 1000, "\n## Test seams\n\n\n## Where to look\n\nhere\n", "a: ## Test seams is empty"},
		{"none with reason", "implement", 1000, "\n## Test seams\n\nnone — pure docs\n" + where, ""},
		{"bare none", "implement", 1000, "\n## Test seams\n\nnone\n" + where, ""},
		{"no where to look", "implement", 1000, "\n## Test seams\n\nnone\n", "a: no ## Where to look"},
		{"over the limit", "implement", ticketspec.SplitLimitTokens + 1, "\n## Test seams\n\nnone\n" + where, fmt.Sprintf("split limit %d", ticketspec.SplitLimitTokens)},
		{"verify exempt from sections", "verify", 1000, "nothing\n", ""},
		{"research exempt from sections", "research", 1000, "nothing\n", ""},
		{"verify exempt from the cap", "verify", ticketspec.SplitLimitTokens + 1, "nothing\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, project, err := publishShape(t, tc.typ, tc.window, tc.body)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("publish: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
			if _, statErr := os.Stat(filepath.Join(project, "e")); !os.IsNotExist(statErr) {
				t.Errorf("epic left behind: %v", statErr)
			}
		})
	}
}

func TestPublishAllowFlagsAreIndependent(t *testing.T) {
	// A shape miss (no Test seams) and a claim miss together.
	body := "see `docs/nowhere.md`\n\n## Where to look\n\nhere\n"
	if _, _, err := publishShape(t, "implement", 1000, body, "--allow-check-misses"); err == nil {
		t.Error("--allow-check-misses overrode a claim miss")
	}
	if _, _, err := publishShape(t, "implement", 1000, body, "--allow-claim-misses"); err == nil {
		t.Error("--allow-claim-misses overrode a check miss")
	}
	if _, _, err := publishShape(t, "implement", 1000, body, "--allow-check-misses", "--allow-claim-misses"); err != nil {
		t.Errorf("both flags: %v", err)
	}
	if _, _, err := publishShape(t, "implement", ticketspec.SplitLimitTokens+1, "\n## Test seams\n\nnone\n\n## Where to look\n\nx\n", "--allow-check-misses"); err != nil {
		t.Errorf("--allow-check-misses over the cap: %v", err)
	}
}

func TestPublishBundledAdvisory(t *testing.T) {
	section := func(criteria int, files int, extra string) string {
		var b strings.Builder
		b.WriteString("\n## Test seams\n\nnone\n\n## Where to look\n\n")
		for i := range files {
			fmt.Fprintf(&b, "- `docs/f%d.md`\n", i)
		}
		b.WriteString("\n## Acceptance criteria\n\n")
		for i := range criteria {
			fmt.Fprintf(&b, "- [ ] criterion %d\n", i)
		}
		b.WriteString(extra)
		return b.String()
	}
	cases := []struct {
		name  string
		body  string
		extra []string
		want  bool
	}{
		{"5 and 5", section(5, 5, ""), nil, true},
		{"5 and 5 dry run", section(5, 5, ""), []string{"--dry-run"}, true},
		{"4 criteria", section(4, 5, ""), nil, false},
		{"4 files", section(5, 4, ""), nil, false},
		{"boilerplate not counted", section(4, 5, "- [ ] Tests pass.\n- [ ] `make verify` is green\n"), nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, _, err := publishShape(t, "implement", 1000, tc.body, append([]string{"--allow-claim-misses"}, tc.extra...)...)
			if err != nil {
				t.Fatalf("publish: %v\n%s", err, publishOut(d))
			}
			out := publishOut(d)
			if got := strings.Contains(out, "consider splitting"); got != tc.want {
				t.Fatalf("advisory printed = %v, want %v:\n%s", got, tc.want, out)
			}
			if tc.want && !strings.Contains(out, "a: 5 acceptance criteria and 5 files") {
				t.Errorf("advisory lacks slug and counts:\n%s", out)
			}
		})
	}
}

func TestPublishStampsCommit(t *testing.T) {
	d, project, err := publishShape(t, "implement", 1000, "verified at {{commit}}, keep \\{{commit}}\n"+shapeSections)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	cwd, _ := d.Getwd()
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(out))
	raw, err := os.ReadFile(filepath.Join(project, "e", "issues", "01-a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "verified at " + commit + ", keep {{commit}}"; !strings.Contains(string(raw), want) {
		t.Errorf("ticket want %q:\n%s", want, raw)
	}
	if !strings.Contains(publishOut(d), commit) {
		t.Errorf("lint header lacks %s:\n%s", commit, publishOut(d))
	}
}
