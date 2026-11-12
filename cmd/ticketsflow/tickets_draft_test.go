package ticketsflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func draftSetup(t *testing.T, plan string) (d Deps, planPath, manifest string) {
	t.Helper()
	d, _, manifest = setupPublish(t)
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
	changed := strings.Replace(draftPlan, "runAlpha", "runAlpha2", 1)
	changed = changed[:strings.Index(changed, "  - slug: gamma")]
	if err := os.WriteFile(plan, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	runDraftCmd(t, d, plan, manifest)

	got := readManifestFile(t, alpha)
	if !strings.Contains(got, "Build the alpha.") || !strings.Contains(got, "runAlpha2") || strings.Count(got, "<!-- TODO -->") != 1 {
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
