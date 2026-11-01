package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const verifySpec = "## User Stories\n\n" +
	"1. As an agent, I want a, so that b.\n" +
	"2. As an agent, I want c, so that\n   d.\n" +
	"3. As an agent, I want e, so that f.\n\n" +
	"## Verification\n\n" +
	"1. Run `echo {{b}}` and check `{{commit}}` is kept.\n" +
	"2. Flow \"x\" at 1100px.\n   Second line.\n\n   After a blank line.\n" +
	"3. Run `go test`.\n\n" +
	"**Mock states** (fixture scenario `demo`)\n\n" +
	"- `$(gx tickets root)/demo/mocks/01.html`, calm, 1100px,\n  parts: top-bar.\n" +
	"- `$(gx tickets root)/demo/mocks/01.html`, busy, 1100px, parts: tray.\n\n" +
	"## Out of Scope\n\n- Nothing {{here}}.\n\n## Further Notes\n\nx\n"

const (
	smallPlan = "scenario: demo\nstories:\n  - story: 1\n    cli_checks: 1\n  - story: 2\n    web_flows: 1\n    mock_states: [calm]\n  - story: 3\n    go_tests: 1\n"
	bigPlan   = "scenario: demo\nstories:\n  - story: 1\n    web_flows: 8\n  - story: 2\n    web_flows: 8\n  - story: 3\n    web_flows: 8\n"
)

// verifyRepo is a project with docs/specs/demo.md, a plan file and a manifest path.
func verifyRepo(t *testing.T, plan string) (d deps, project, dir, planPath, manifest string) {
	t.Helper()
	d, project, manifest = setupPublish(t)
	cwd, _ := d.getwd()
	if err := os.MkdirAll(filepath.Join(cwd, "docs", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "docs", "specs", "demo.md"), []byte(verifySpec), 0o644); err != nil {
		t.Fatal(err)
	}
	dir = filepath.Dir(manifest)
	planPath = filepath.Join(dir, "plan.yaml")
	if err := os.WriteFile(planPath, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	return d, project, dir, planPath, manifest
}

func TestVerifyPlan_TablePrintsEstimates(t *testing.T) {
	out, err := runIn(t, t.TempDir(), "tickets", "verify-plan", "--table")
	if err != nil {
		t.Fatalf("--table: %v", err)
	}
	wantLines(t, out, "web flow", "6K", "CLI check", "3K", "Go test run", "4K", "skill quote", "2K",
		"mock state", "8K", "part base", "15K", "collect estimate", "30K", "split limit", "60K")
}

func TestVerifyPlan_PrintsEntries(t *testing.T) {
	d, _, _, plan, _ := verifyRepo(t, "stories:\n  - story: 1\n    web_flows: 8\n  - story: 2\n    web_flows: 1\n")
	if err := execute([]string{"tickets", "verify-plan", "mine:demo-impl", "--plan", plan}, d); err != nil {
		t.Fatalf("verify-plan: %v", err)
	}
	wantLines(t, publishOut(d), "verify-part-1: window=63K", "over budget", "verify-part-2:", "verify-collect: window=30K blocked_by=verify-part-1,verify-part-2")
}

func TestVerifyPlan_BadPlanFails(t *testing.T) {
	d, _, dir, _, _ := verifyRepo(t, "stories: []\n")
	empty := filepath.Join(dir, "plan.yaml")
	if err := execute([]string{"tickets", "verify-plan", "mine:demo-impl", "--plan", empty}, d); err == nil {
		t.Error("empty plan: want error")
	}
	if err := execute([]string{"tickets", "verify-plan", "mine:demo-impl", "--plan", filepath.Join(dir, "missing.yaml")}, d); err == nil {
		t.Error("missing plan: want error")
	}
	if err := execute([]string{"tickets", "verify-plan", "mine:demo-impl"}, d); err == nil {
		t.Error("no --plan: want error")
	}
}

func readManifestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestVerifyPlan_OnePartWritesLoneEntry(t *testing.T) {
	d, _, dir, plan, _ := verifyRepo(t, smallPlan)
	if err := execute([]string{"tickets", "verify-plan", "mine:demo-impl", "--plan", plan, "--manifest", filepath.Join(dir, "verify.yaml")}, d); err != nil {
		t.Fatalf("verify-plan: %v", err)
	}
	got := readManifestFile(t, filepath.Join(dir, "verify.yaml"))
	if !strings.Contains(got, "slug: verify\n") || strings.Contains(got, "verify-collect") || strings.Contains(got, "verify-part") {
		t.Errorf("manifest:\n%s", got)
	}
	body := readManifestFile(t, filepath.Join(dir, "verify-body-verify.md"))
	for _, want := range []string{"Round: 1\n", "docs/specs/demo.md", "Fixture scenario: demo", "## Out of Scope", "(estimate 8K)"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Role:") {
		t.Errorf("lone ticket has a Role line:\n%s", body)
	}
}

func TestVerifyPlan_SeveralPartsWriteCollect(t *testing.T) {
	d, _, dir, plan, _ := verifyRepo(t, bigPlan)
	m := filepath.Join(dir, "verify.yaml")
	if err := execute([]string{"tickets", "verify-plan", "mine:demo-impl", "--plan", plan, "--manifest", m}, d); err != nil {
		t.Fatalf("verify-plan: %v", err)
	}
	got := readManifestFile(t, m)
	for _, slug := range []string{"verify-part-1", "verify-part-2", "verify-part-3", "verify-collect"} {
		if !strings.Contains(got, "slug: "+slug+"\n") {
			t.Errorf("manifest missing %s:\n%s", slug, got)
		}
	}
	if strings.Contains(got, "parent") {
		t.Errorf("manifest has a parent:\n%s", got)
	}
	if n := strings.Count(got, "blocked_by:"); n != 1 {
		t.Errorf("only the collect entry has blockers, got %d:\n%s", n, got)
	}
	collect := readManifestFile(t, filepath.Join(dir, "verify-body-verify-collect.md"))
	if !strings.Contains(collect, "Role: collect") || !strings.Contains(collect, "Nothing") {
		t.Errorf("collect body:\n%s", collect)
	}
	part := readManifestFile(t, filepath.Join(dir, "verify-body-verify-part-1.md"))
	if !strings.Contains(part, "Role: part") || strings.Contains(part, "Out of Scope") {
		t.Errorf("part body:\n%s", part)
	}
}

func TestVerifyPlan_MissingStoryFails(t *testing.T) {
	d, _, dir, plan, _ := verifyRepo(t, "stories:\n  - story: 9\n    cli_checks: 1\n")
	m := filepath.Join(dir, "verify.yaml")
	err := execute([]string{"tickets", "verify-plan", "mine:demo-impl", "--plan", plan, "--manifest", m}, d)
	if err == nil || !strings.Contains(err.Error(), "story 9") {
		t.Fatalf("err = %v, want a story 9 error", err)
	}
	if _, statErr := os.Stat(m); !os.IsNotExist(statErr) {
		t.Errorf("manifest written despite error: %v", statErr)
	}
}

func TestVerifyPlan_MissingMockStateFails(t *testing.T) {
	d, _, dir, plan, _ := verifyRepo(t, "stories:\n  - story: 1\n    mock_states: [nosuch]\n")
	err := execute([]string{"tickets", "verify-plan", "mine:demo-impl", "--plan", plan, "--manifest", filepath.Join(dir, "v.yaml")}, d)
	if err == nil || !strings.Contains(err.Error(), "nosuch") {
		t.Fatalf("err = %v, want a nosuch error", err)
	}
}

// Seams 2 and 3: the copy is word for word after publish, {{commit}} is not
// stamped, implement and verify tickets publish in one call, and ensure-verify
// adds nothing.
func TestVerifyPlan_PublishesWithImplementTickets(t *testing.T) {
	d, project, _, plan, manifest := verifyRepo(t, smallPlan)
	if err := execute([]string{"tickets", "verify-plan", "mine:demo-impl", "--plan", plan, "--manifest", manifest}, d); err != nil {
		t.Fatalf("verify-plan: %v", err)
	}
	if err := execute([]string{"tickets", "publish", "mine:demo-impl", "--manifest", manifest, "--allow-check-misses"}, d); err != nil {
		t.Fatalf("publish: %v", err)
	}
	names := strings.Join(issueNames(t, project, "demo-impl"), " ")
	if names != "01-a.md 02-b.md 03-c.md 04-verify.md" {
		t.Fatalf("issues = %s", names)
	}
	raw := readManifestFile(t, filepath.Join(project, "demo-impl", "issues", "04-verify.md"))
	want := "1. Run `echo {{b}}` and check `{{commit}}` is kept."
	if !strings.Contains(raw, want) {
		t.Errorf("verify ticket lacks the word-for-word entry:\n%s", raw)
	}
	if err := execute([]string{"tickets", "ensure-verify", "mine:demo-impl"}, d); err != nil {
		t.Fatalf("ensure-verify: %v", err)
	}
	if got := strings.Join(issueNames(t, project, "demo-impl"), " "); got != names {
		t.Errorf("ensure-verify changed issues: %s", got)
	}
}
