package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/tickets/schema"
)

// addFollowUp files a draft follow-up into gx:follow-ups of the fixture's store.
func addFollowUp(t *testing.T, f *evolveFixture, file, id, extraFM, body string) string {
	t.Helper()
	epic := filepath.Join(f.env.storePath, "gx", "follow-ups")
	if err := os.MkdirAll(filepath.Join(epic, "issues"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(epic, "ticket.md"), []byte("---\nstatus: draft\n---\n\n# follow-ups\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(epic, "issues", file)
	content := "---\nid: \"" + id + "\"\nstatus: draft\ntype: research\n" + extraFM + "---\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func twoFollowUps(t *testing.T) (*evolveFixture, string, string) {
	f := newEvolveFixture(t, config.EvolveContributor, true, "")
	a := addFollowUp(t, f, "01-first-problem.md", "01", "category: bug\n",
		"## Source\n\nproj:epic/03\n\n## Problem\n\nIt breaks.\n\n```\n# not a heading\n```\n\n## Suggested fix\n\nFix it.\n\n## Evidence\n\nLog.\n")
	b := addFollowUp(t, f, "02-second-problem.md", "02", "category: idea\n",
		"## Source\n\nother:epic/01\n\n## Problem\n\nCould be better.\n\n## Suggested fix\n\nImprove.\n")
	return f, a, b
}

func exportedAt(t *testing.T, path string) bool {
	t.Helper()
	sc, err := schema.ParseTicket(path)
	if err != nil {
		t.Fatal(err)
	}
	return !sc.ExportedAt.IsZero()
}

func TestEvolveExport_BundleShapeAndStamp(t *testing.T) {
	t.Parallel()
	f, a, b := twoFollowUps(t)
	var out, errOut bytes.Buffer
	if err := runEvolveExport(f.env, &out, &errOut, "", false); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"# gx follow-ups — 2026-03-04\n",
		"## First problem\n\n```\ncategory: bug\ntype: research\nsource: proj:epic/03\n```\n",
		"### Source\n",
		"### Suggested fix\n",
		"# not a heading\n", // inside a fence: left alone
		"## Second problem\n\n```\ncategory: idea\ntype: research\nsource: other:epic/01\n```\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("bundle misses %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "## not a heading") {
		t.Errorf("fenced line was pushed down:\n%s", got)
	}
	if !exportedAt(t, a) || !exportedAt(t, b) {
		t.Error("exported_at not stamped on both")
	}

	out.Reset()
	if err := runEvolveExport(f.env, &out, &errOut, "", false); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("second run exported again:\n%s", out.String())
	}

	if err := runEvolveExport(f.env, &out, &errOut, "", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "## First problem") || !strings.Contains(out.String(), "## Second problem") {
		t.Errorf("--again did not re-export:\n%s", out.String())
	}
}

func TestEvolveExport_OutputFile(t *testing.T) {
	t.Parallel()
	f, a, _ := twoFollowUps(t)
	file := filepath.Join(t.TempDir(), "bundle.md")
	var out, errOut bytes.Buffer
	if err := runEvolveExport(f.env, &out, &errOut, file, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil || !strings.HasPrefix(string(data), "# gx follow-ups — ") {
		t.Fatalf("bundle file = %q, %v", data, err)
	}
	if out.Len() != 0 {
		t.Errorf("-o also wrote to stdout: %q", out.String())
	}
	if !exportedAt(t, a) {
		t.Error("exported_at not stamped")
	}
}

func TestEvolveReminder_EveryVerbInContributorMode(t *testing.T) {
	t.Parallel()
	f, _, _ := twoFollowUps(t)
	const want = "2 unexported follow-ups; run `gx evolve export`\n"

	verbs := map[string]func(out *bytes.Buffer) error{
		"run": func(out *bytes.Buffer) error {
			return runEvolveRun(context.Background(), f.env, out, &bytes.Buffer{}, "proj:epic", true)
		},
		"skip": func(out *bytes.Buffer) error { return runEvolveSkip(f.env, out, "proj:epic") },
		"inputs": func(out *bytes.Buffer) error {
			return runEvolveInputs(f.env, out, "proj:epic", false)
		},
		"all": func(out *bytes.Buffer) error {
			return runEvolveAll(context.Background(), f.env, out, &bytes.Buffer{}, evolveAllWindow, true, true)
		},
	}
	for name, run := range verbs {
		var out bytes.Buffer
		if err := run(&out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(out.String(), want) {
			t.Errorf("%s output misses the reminder:\n%s", name, out.String())
		}
	}

	if err := runEvolveExport(f.env, &bytes.Buffer{}, &bytes.Buffer{}, "", false); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runEvolveSkip(f.env, &out, "proj:epic"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "unexported") {
		t.Errorf("reminder printed with nothing unexported:\n%s", out.String())
	}
}

func TestEvolveReminder_SilentOutsideContributorMode(t *testing.T) {
	t.Parallel()
	f := newEvolveFixture(t, config.EvolveMaintainer, true, "")
	addFollowUp(t, f, "01-x.md", "01", "category: bug\n", "## Source\n\ns\n")
	var out bytes.Buffer
	if err := runEvolveSkip(f.env, &out, "proj:epic"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "unexported") {
		t.Errorf("maintainer mode printed the reminder:\n%s", out.String())
	}
}
