package ticketsflow

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/migrations"
)

const draftPlan = `scenario: demo
tickets:
  - slug: alpha
    title: Alpha thing
    window: 40000
    entry_symbols: [runAlpha]
    test_files: [cmd/alpha_test.go]
    mock_states: ["$(gx tickets root)/demo/mocks/01.html, calm, 1100px, parts: top-bar"]
    changed_symbols: [AlphaFn]
    changed_fields: [Alpha.Field]
  - slug: beta
    title: Beta thing
    blocked_by: [alpha]
    window: 40000
    entry_symbols: [runBeta]
    test_files: [cmd/beta_test.go]
    mock_states: ["$(gx tickets root)/demo/mocks/01.html, busy, 1100px, parts: tray"]
  - slug: gamma
    title: Gamma thing
    window: 40000
    entry_symbols: [runGamma]
    test_files: [cmd/gamma_test.go]
    mock_states: ["$(gx tickets root)/demo/mocks/02.html, calm, 800px, parts: list"]
`

// draftFixture puts the files and symbols that draftPlan names into the repo.
func draftFixture(t *testing.T, d Deps) {
	t.Helper()
	cwd, _ := d.Getwd()
	if err := os.MkdirAll(filepath.Join(cwd, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package cmd\n\nfunc runAlpha() {}\nfunc runBeta() {}\nfunc runGamma() {}\nfunc AlphaFn() {}\n\nfunc callerOfAlpha() { runAlpha() }\n"
	files := map[string]string{"sym.go": src}
	for _, n := range []string{"alpha", "beta", "gamma"} {
		files[n+"_test.go"] = "package cmd\n\nimport \"testing\"\n\nfunc Test" + strings.ToUpper(n[:1]) + n[1:] + "(t *testing.T) {}\n"
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(cwd, "cmd", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// onlyTools sets PATH to a directory holding just the named tools, plus a
// fake gopls when fakeGopls is set.
func onlyTools(t *testing.T, fakeGopls bool, tools ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, tool := range tools {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s not on PATH", tool)
		}
		if err := os.Symlink(p, filepath.Join(dir, tool)); err != nil {
			t.Fatal(err)
		}
	}
	if fakeGopls {
		script := "#!/bin/sh\necho \"$PWD/cmd/sym.go:8:22-30\"\n"
		if err := os.WriteFile(filepath.Join(dir, "gopls"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func draftOut(t *testing.T, d Deps, plan, manifest string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	d.Env.Stdout = &buf
	err := execute(t, []string{"tickets", "draft", "mine:demo-impl", "--plan", plan, "--manifest", manifest}, d)
	return buf.String(), err
}

func draftSetup(t *testing.T, plan string) (d Deps, planPath, manifest string) {
	t.Helper()
	d, _, manifest = setupPublish(t)
	draftFixture(t, d)
	onlyTools(t, false, "rg", "git") // never a real gopls: it takes seconds per run
	planPath = filepath.Join(filepath.Dir(manifest), "plan.yaml")
	if err := os.WriteFile(planPath, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest = filepath.Join(filepath.Dir(manifest), "draft.yaml")
	return d, planPath, manifest
}

func runDraftCmd(t *testing.T, d Deps, plan, manifest string) {
	t.Helper()
	if err := execute(t, []string{"tickets", "draft", "mine:demo-impl", "--plan", plan, "--manifest", manifest}, d); err != nil {
		t.Fatalf("draft: %v", err)
	}
}

func TestTicketsDraft_WritesManifestAndBodies(t *testing.T) {
	d, plan, manifest := draftSetup(t, draftPlan)
	runDraftCmd(t, d, plan, manifest)
	m := readManifestFile(t, manifest)
	for _, want := range []string{"slug: alpha\n", "slug: beta\n", "slug: gamma\n", "body: draft-body-beta.md", "- alpha"} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest lacks %q:\n%s", want, m)
		}
	}
	for _, slug := range []string{"alpha", "beta", "gamma"} {
		body := readManifestFile(t, filepath.Join(filepath.Dir(manifest), "draft-body-"+slug+".md"))
		for _, want := range []string{"# " + strings.ToUpper(slug[:1]) + slug[1:] + " thing", "## Test seams", "## Where to look", "## Mock check", "Fixture scenario: demo", "cmd/" + slug + "_test.go", "run" + strings.ToUpper(slug[:1]) + slug[1:]} {
			if !strings.Contains(body, want) {
				t.Errorf("%s body lacks %q:\n%s", slug, want, body)
			}
		}
		if n := strings.Count(body, "<!-- TODO -->"); n != 2 {
			t.Errorf("%s body has %d markers, want 2", slug, n)
		}
	}
}

func TestTicketsDraft_RerunKeepsAgentTextAndDeletesRemovedSlug(t *testing.T) {
	d, plan, manifest := draftSetup(t, draftPlan)
	runDraftCmd(t, d, plan, manifest)
	dir := filepath.Dir(manifest)
	alpha := filepath.Join(dir, "draft-body-alpha.md")
	edited := strings.Replace(readManifestFile(t, alpha), "<!-- TODO -->", "Build the alpha.", 1)
	if err := os.WriteFile(alpha, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(draftPlan, "runAlpha", "runGamma", 1)
	changed = changed[:strings.Index(changed, "  - slug: gamma")]
	if err := os.WriteFile(plan, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	runDraftCmd(t, d, plan, manifest)

	got := readManifestFile(t, alpha)
	if !strings.Contains(got, "Build the alpha.") || !strings.Contains(got, "runGamma") || strings.Count(got, "<!-- TODO -->") != 1 {
		t.Errorf("alpha after re-run:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "draft-body-gamma.md")); !os.IsNotExist(err) {
		t.Errorf("gamma body file still exists: %v", err)
	}
	if m := readManifestFile(t, manifest); strings.Contains(m, "gamma") || strings.Count(m, "slug: alpha") != 1 {
		t.Errorf("manifest after re-run:\n%s", m)
	}
}

func TestTicketsDraft_AppendKeepsVerifyEntries(t *testing.T) {
	d, _, dir, vplan, _ := verifyRepo(t, smallPlan)
	draftFixture(t, d)
	onlyTools(t, false, "rg", "git")
	m := filepath.Join(dir, "both.yaml")
	if err := execute(t, []string{"tickets", "verify-plan", "mine:demo-impl", "--plan", vplan, "--manifest", m}, d); err != nil {
		t.Fatalf("verify-plan: %v", err)
	}
	dplan := filepath.Join(dir, "draft-plan.yaml")
	if err := os.WriteFile(dplan, []byte(draftPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	runDraftCmd(t, d, dplan, m)
	got := readManifestFile(t, m)
	for _, want := range []string{"slug: verify\n", "slug: alpha\n", "slug: gamma\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("manifest lacks %q:\n%s", want, got)
		}
	}
	runDraftCmd(t, d, dplan, m)
	if again := readManifestFile(t, m); strings.Count(again, "slug: verify\n") != 1 || strings.Count(again, "slug: alpha\n") != 1 {
		t.Errorf("re-run duplicated rows:\n%s", again)
	}
}

func TestTicketsDraft_BadPlanFails(t *testing.T) {
	d, plan, manifest := draftSetup(t, "tickets: []\n")
	if err := execute(t, []string{"tickets", "draft", "mine:demo-impl", "--plan", plan, "--manifest", manifest}, d); err == nil {
		t.Error("empty plan: want error")
	}
	if err := execute(t, []string{"tickets", "draft", "mine:demo-impl", "--plan", plan}, d); err == nil {
		t.Error("no --manifest: want error")
	}
}

func TestTicketsDraft_MissingNamesFail(t *testing.T) {
	for name, tc := range map[string]struct{ old, new, want string }{
		"path":    {"cmd/alpha_test.go", "cmd/nope_test.go", "no such file"},
		"symbol":  {"runBeta", "runMissing", "no such symbol"},
		"pattern": {"calm, 1100px", "calm, 1100px, `-run 'TestNoSuch'`", "matches no Test function"},
		"flow":    {"Alpha thing", "Alpha thing for flow 9999", "flow 9999"},
	} {
		t.Run(name, func(t *testing.T) {
			d, plan, manifest := draftSetup(t, strings.Replace(draftPlan, tc.old, tc.new, 1))
			if name == "flow" {
				cwd, _ := d.Getwd()
				_ = os.MkdirAll(filepath.Join(cwd, "webui", "e2e"), 0o755)
				_ = os.WriteFile(filepath.Join(cwd, "webui", "e2e", "CATALOG.md"), []byte("## 1. one\n"), 0o644)
			}
			out, err := draftOut(t, d, plan, manifest)
			if err == nil || !strings.Contains(out, "miss: ") || !strings.Contains(out, tc.want) {
				t.Errorf("err=%v out:\n%s", err, out)
			}
		})
	}
}

func TestTicketsDraft_GoodPlanStampsCallersFromGopls(t *testing.T) {
	d, plan, manifest := draftSetup(t, draftPlan)
	onlyTools(t, true, "rg", "git")
	out, err := draftOut(t, d, plan, manifest)
	if err != nil {
		t.Fatalf("draft: %v\n%s", err, out)
	}
	body := readManifestFile(t, filepath.Join(filepath.Dir(manifest), "draft-body-alpha.md"))
	if !strings.Contains(body, "callers (verified at ") || !strings.Contains(body, "cmd/sym.go:8") {
		t.Errorf("body:\n%s", body)
	}
	if !strings.Contains(out, "draft took ") {
		t.Errorf("no wall-clock line:\n%s", out)
	}
}

func TestTicketsDraft_NoGoplsFallsBackToRg(t *testing.T) {
	d, plan, manifest := draftSetup(t, draftPlan)
	onlyTools(t, false, "rg", "git")
	out, err := draftOut(t, d, plan, manifest)
	if err != nil {
		t.Fatalf("draft: %v\n%s", err, out)
	}
	body := readManifestFile(t, filepath.Join(filepath.Dir(manifest), "draft-body-alpha.md"))
	if !strings.Contains(body, "callers (rg result, not verified: ") || strings.Contains(body, "verified at ") {
		t.Errorf("body:\n%s", body)
	}
	if strings.Contains(strings.ToLower(out), "judg") {
		t.Errorf("output claims a judgment check:\n%s", out)
	}
}

const factsPlan = `tickets:
  - slug: facts
    title: Facts thing
    window: 40000
    entry_symbols: [runAlpha]
    test_files: [webui/src/node.test.ts, webui/src/dom.test.ts]
    make_targets: [web-types]
    helpers: [AlphaFn]
    migrations: true
    changed_fields: [Alpha.Field]
    changed_symbols: [AlphaFn]
    playwright:
      - {file: flow.e2e.ts, flows: [7], grep: "Seven"}
`

func factsFixture(t *testing.T, d Deps) {
	t.Helper()
	cwd, _ := d.Getwd()
	for name, content := range map[string]string{
		"Makefile":               "web-types:\n\ttrue\n",
		"webui/src/node.test.ts": "// @vitest-environment node\nimport 'x'\n",
		"webui/src/dom.test.ts":  "import 'x'\n",
		"webui/e2e/flow.e2e.ts":  "test(\"7. Seven things\", async () => {})\n",
		"cmd/field.go":           "package cmd\n\ntype Alpha struct{ Field int }\n\nfunc useField(a Alpha) int { return a.Field }\n",
		"cmd/field_test.go":      "package cmd\n\nimport \"testing\"\n\nfunc TestField(t *testing.T) { _ = Alpha{Field: 1} }\n",
	} {
		p := filepath.Join(cwd, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTicketsDraft_PrintsEnvironmentFacts(t *testing.T) {
	d, plan, manifest := draftSetup(t, factsPlan)
	factsFixture(t, d)
	out, err := draftOut(t, d, plan, manifest)
	if err != nil {
		t.Fatalf("draft: %v\n%s", err, out)
	}
	body := readManifestFile(t, filepath.Join(filepath.Dir(manifest), "draft-body-facts.md"))
	for _, want := range []string{
		"test environment of `webui/src/node.test.ts`: node",
		"test environment of `webui/src/dom.test.ts`: default",
		"make target `web-types` exists",
		"highest migration number in use: ",
		"helper `AlphaFn` at cmd/sym.go:6",
		"playwright -g `Seven` on flow.e2e.ts: finds 1 test(s): 7. Seven things",
		"consumers (rg, not verified: cmd/field.go:",
		"existing tests mentioning it (rg, not verified: cmd/field_test.go:",
		"callers (rg result, not verified: ",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
}

func TestTicketsDraft_VerifiedAtCallers(t *testing.T) {
	d, plan, manifest := draftSetup(t, factsPlan)
	factsFixture(t, d)
	onlyTools(t, true, "rg", "git")
	if out, err := draftOut(t, d, plan, manifest); err != nil {
		t.Fatalf("draft: %v\n%s", err, out)
	}
	body := readManifestFile(t, filepath.Join(filepath.Dir(manifest), "draft-body-facts.md"))
	if !strings.Contains(body, "callers (verified at ") {
		t.Errorf("body:\n%s", body)
	}
}

func TestTicketsDraft_MissingMakeTargetAndAnchoredPatternFail(t *testing.T) {
	plan := strings.Replace(factsPlan, "[web-types]", "[no-such-target]", 1)
	plan = strings.Replace(plan, `"Seven"`, `"^7"`, 1)
	d, planPath, manifest := draftSetup(t, plan)
	factsFixture(t, d)
	out, err := draftOut(t, d, planPath, manifest)
	if err == nil {
		t.Fatalf("draft passed:\n%s", out)
	}
	for _, want := range []string{"make target `no-such-target` is not in Makefile", "finds no tests"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestTicketsDraft_MigrationLineMatchesRegistry(t *testing.T) {
	d, plan, manifest := draftSetup(t, factsPlan)
	factsFixture(t, d)
	runDraftCmd(t, d, plan, manifest)
	body := readManifestFile(t, filepath.Join(filepath.Dir(manifest), "draft-body-facts.md"))
	names := migrations.Default.Names()
	want := fmt.Sprintf("highest migration number in use: %d (step %s)", len(names), names[len(names)-1])
	if !strings.Contains(body, want) {
		t.Errorf("body lacks %q:\n%s", want, body)
	}
}

func TestTicketsDraft_MarkerBodyRefusedByPublishAndLint(t *testing.T) {
	d, plan, manifest := draftSetup(t, draftPlan)
	runDraftCmd(t, d, plan, manifest)
	err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest, "--dry-run"}, d)
	if err == nil || !strings.Contains(err.Error(), "<!-- TODO -->") {
		t.Fatalf("publish --dry-run err = %v, want a marker refusal", err)
	}
	if err := execute(t, []string{"tickets", "publish", "mine:e", "--manifest", manifest, "--dry-run", "--allow-check-misses"}, d); err == nil {
		t.Error("--allow-check-misses must not skip the marker check")
	}
}
