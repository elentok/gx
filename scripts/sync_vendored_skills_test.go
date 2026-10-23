package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// syncFixture is a fixture upstream repo, a gx copy of one of its skills and a
// manifest pointing at it. Later sync-script tests reuse it.
type syncFixture struct {
	base     string // GX_UPSTREAM_BASE: upstream repos live at <base>/<repo>
	upstream string // the upstream work tree, <base>/<repo>
	gxSkill  string // gx's copy of the skill
	manifest string
	pin      string // upstream commit the manifest pins
}

const fixtureRepo = "owner/skills"

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commitUpstream writes files (path -> content) into the upstream work tree and commits.
func (f *syncFixture) commitUpstream(t *testing.T, msg string, files map[string]string) string {
	t.Helper()
	for p, c := range files {
		writeFile(t, filepath.Join(f.upstream, p), c)
	}
	gitIn(t, f.upstream, "add", "-A")
	gitIn(t, f.upstream, "commit", "-m", msg)
	return gitIn(t, f.upstream, "rev-parse", "HEAD")
}

// newSyncFixture builds an upstream repo with skills "alpha" (pinned after its
// first commit) and "beta", then a manifest with one entry per skill.
func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	root := t.TempDir()
	f := &syncFixture{
		base:     filepath.Join(root, "upstream"),
		gxSkill:  filepath.Join(root, "gx", "gx-alpha"),
		manifest: filepath.Join(root, "manifest"),
	}
	f.upstream = filepath.Join(f.base, fixtureRepo)
	if err := os.MkdirAll(f.upstream, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, f.upstream, "init", "-q", "-b", "main")
	f.pin = f.commitUpstream(t, "initial", map[string]string{
		"LICENSE":               "MIT\n",
		"skills/alpha/SKILL.md": "alpha v1\n",
		"skills/beta/SKILL.md":  "beta v1\n",
	})
	writeFile(t, filepath.Join(f.gxSkill, "SKILL.md"), "alpha v1\n")
	writeFile(t, f.manifest,
		"gx-alpha "+fixtureRepo+" skills/alpha LICENSE "+f.pin+"\n"+
			"gx-beta "+fixtureRepo+" skills/beta LICENSE "+f.pin+" port\n")
	return f
}

func (f *syncFixture) runSync(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"sync-vendored-skills.sh", "--manifest", f.manifest, "--skills-dir", filepath.Dir(f.gxSkill)}, args...)...)
	cmd.Env = append(os.Environ(), "GX_UPSTREAM_BASE="+f.base)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sync script: %v\n%s", err, out)
	}
	return string(out)
}

// snapshot lists every file under dir with its content, to assert nothing was written.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.Contains(p, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			return err
		}
		b, err := os.ReadFile(p)
		got[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestSyncCheck_ReportsCommitCountsPerSkill(t *testing.T) {
	f := newSyncFixture(t)
	f.commitUpstream(t, "alpha 2", map[string]string{"skills/alpha/SKILL.md": "alpha v2\n"})
	f.commitUpstream(t, "alpha 3", map[string]string{"skills/alpha/extra.md": "x\n"})
	f.commitUpstream(t, "unrelated", map[string]string{"skills/other/SKILL.md": "o\n"})

	out := f.runSync(t, "--check")

	if !strings.Contains(out, "gx-alpha: 2 new upstream commits") {
		t.Errorf("want alpha with 2 commits, got:\n%s", out)
	}
	if strings.Contains(out, "gx-beta") {
		t.Errorf("beta has no new commits and must not be listed, got:\n%s", out)
	}
}

func TestSyncCheck_ListsNothingWhenUpToDate(t *testing.T) {
	f := newSyncFixture(t)

	out := f.runSync(t, "--check")

	if strings.Contains(out, "gx-alpha") || strings.Contains(out, "gx-beta") {
		t.Errorf("no skill should be listed, got:\n%s", out)
	}
}

func TestSyncCheck_WritesNothing(t *testing.T) {
	f := newSyncFixture(t)
	f.commitUpstream(t, "alpha 2", map[string]string{"skills/alpha/SKILL.md": "alpha v2\n"})
	manifestBefore, _ := os.ReadFile(f.manifest)
	gxBefore := snapshot(t, filepath.Dir(f.gxSkill))
	upstreamHead := gitIn(t, f.upstream, "rev-parse", "HEAD")
	upstreamStatus := gitIn(t, f.upstream, "status", "--porcelain")

	f.runSync(t, "--check")

	manifestAfter, _ := os.ReadFile(f.manifest)
	if string(manifestBefore) != string(manifestAfter) {
		t.Error("manifest changed")
	}
	gxAfter := snapshot(t, filepath.Dir(f.gxSkill))
	if len(gxBefore) != len(gxAfter) {
		t.Errorf("gx skill files changed: %v -> %v", gxBefore, gxAfter)
	}
	for p, c := range gxBefore {
		if gxAfter[p] != c {
			t.Errorf("%s changed", p)
		}
	}
	if gitIn(t, f.upstream, "rev-parse", "HEAD") != upstreamHead || gitIn(t, f.upstream, "status", "--porcelain") != upstreamStatus {
		t.Error("upstream repo changed")
	}
}

// newMergeFixture extends the base fixture with a multi-file alpha skill, pinned at its first state.
func newMergeFixture(t *testing.T) *syncFixture {
	t.Helper()
	f := newSyncFixture(t)
	f.pin = f.commitUpstream(t, "alpha files", map[string]string{
		"skills/alpha/a.md":      "1\n2\n3\n4\n5\n6\n7\n",
		"skills/alpha/gone.md":   "gone\n",
		"skills/alpha/gxgone.md": "gxgone\n",
		"skills/alpha/README.md": "readme v1\n",
	})
	for name, c := range map[string]string{
		"a.md": "1\n2\n3\n4\n5\n6\n7\n", "gone.md": "gone\n", "README.md": "readme v1\n",
	} {
		writeFile(t, filepath.Join(f.gxSkill, name), c)
	}
	writeFile(t, f.manifest, "gx-alpha "+fixtureRepo+" skills/alpha LICENSE "+f.pin+"\n")
	return f
}

func readGx(t *testing.T, f *syncFixture, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.gxSkill, name))
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func TestSyncMerge_CleanMergeKeepsGxEditsAndTakesUpstreamChanges(t *testing.T) {
	f := newMergeFixture(t)
	writeFile(t, filepath.Join(f.gxSkill, "a.md"), "1\n2\n3\n4\n5\n6\ngx\n")
	f.commitUpstream(t, "edit top", map[string]string{
		"skills/alpha/a.md": "up\n2\n3\n4\n5\n6\n7\n", "skills/alpha/README.md": "readme v2\n",
	})

	out := f.runSync(t, "--merge")

	if got := readGx(t, f, "a.md"); got != "up\n2\n3\n4\n5\n6\ngx\n" {
		t.Errorf("a.md = %q", got)
	}
	if got := readGx(t, f, "README.md"); got != "readme v1\n" {
		t.Errorf("README.md must be untouched, got %q", got)
	}
	for _, want := range []string{"1 new upstream commits", "edit top", "+up", "clean files:\n  a.md\n", "conflicted files:\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "conflicted files:\n  ") {
		t.Errorf("nothing should conflict:\n%s", out)
	}
}

func TestSyncMerge_ConflictLeavesMarkersAndListsFile(t *testing.T) {
	f := newMergeFixture(t)
	writeFile(t, filepath.Join(f.gxSkill, "a.md"), "gx\n2\n3\n4\n5\n6\n7\n")
	f.commitUpstream(t, "edit top", map[string]string{"skills/alpha/a.md": "up\n2\n3\n4\n5\n6\n7\n"})

	out := f.runSync(t, "--merge")

	if got := readGx(t, f, "a.md"); !strings.Contains(got, "<<<<<<<") || !strings.Contains(got, ">>>>>>>") {
		t.Errorf("want conflict markers, got %q", got)
	}
	if !strings.Contains(out, "conflicted files:\n  a.md\n") {
		t.Errorf("a.md should be conflicted:\n%s", out)
	}
}

func TestSyncMerge_UpstreamAddedFileIsCopiedIn(t *testing.T) {
	f := newMergeFixture(t)
	f.commitUpstream(t, "add", map[string]string{"skills/alpha/sub/new.md": "new\n"})

	out := f.runSync(t, "--merge")

	if got := readGx(t, f, "sub/new.md"); got != "new\n" {
		t.Errorf("new.md = %q", got)
	}
	if !strings.Contains(out, "clean files:\n  sub/new.md\n") {
		t.Errorf("report:\n%s", out)
	}
}

func TestSyncMerge_UpstreamDeletedFile(t *testing.T) {
	for _, tc := range []struct {
		name       string
		gxEdit     bool
		wantExists bool
		wantList   string
	}{
		{"unchanged in gx is deleted", false, false, "clean files:\n  gone.md\n"},
		{"changed in gx conflicts", true, true, "conflicted files:\n  gone.md\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMergeFixture(t)
			if tc.gxEdit {
				writeFile(t, filepath.Join(f.gxSkill, "gone.md"), "gx edit\n")
			}
			gitIn(t, f.upstream, "rm", "-q", "skills/alpha/gone.md")
			gitIn(t, f.upstream, "commit", "-q", "-m", "delete")

			out := f.runSync(t, "--merge")

			if exists := readGx(t, f, "gone.md") != "<missing>"; exists != tc.wantExists {
				t.Errorf("exists = %v, want %v", exists, tc.wantExists)
			}
			if !strings.Contains(out, tc.wantList) {
				t.Errorf("report missing %q:\n%s", tc.wantList, out)
			}
		})
	}
}

func TestSyncMerge_GxDeletedFile(t *testing.T) {
	// gxgone.md is in base but gx never had it.
	t.Run("upstream unchanged stays deleted", func(t *testing.T) {
		f := newMergeFixture(t)
		f.commitUpstream(t, "other", map[string]string{"skills/alpha/a.md": "up\n2\n3\n4\n5\n6\n7\n"})

		out := f.runSync(t, "--merge")

		if got := readGx(t, f, "gxgone.md"); got != "<missing>" {
			t.Errorf("gxgone.md came back: %q", got)
		}
		if strings.Contains(out, "gxgone.md") {
			t.Errorf("gxgone.md should not be reported:\n%s", out)
		}
	})
	t.Run("upstream changed conflicts", func(t *testing.T) {
		f := newMergeFixture(t)
		f.commitUpstream(t, "edit", map[string]string{"skills/alpha/gxgone.md": "gxgone v2\n"})

		out := f.runSync(t, "--merge")

		if got := readGx(t, f, "gxgone.md"); got != "<missing>" {
			t.Errorf("gxgone.md written: %q", got)
		}
		if !strings.Contains(out, "conflicted files:\n  gxgone.md\n") {
			t.Errorf("report:\n%s", out)
		}
	})
}

func TestSyncMerge_DoesNotMoveThePin(t *testing.T) {
	f := newMergeFixture(t)
	f.commitUpstream(t, "edit", map[string]string{"skills/alpha/a.md": "up\n2\n3\n4\n5\n6\n7\n"})
	before, _ := os.ReadFile(f.manifest)

	f.runSync(t, "--merge")

	after, _ := os.ReadFile(f.manifest)
	if string(before) != string(after) {
		t.Error("manifest changed")
	}
}

func assertSnapshotUnchanged(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Errorf("files changed: %v -> %v", before, after)
	}
	for p, c := range before {
		if after[p] != c {
			t.Errorf("%s changed", p)
		}
	}
}

func TestSyncPort_PrintsTheDiffAndWritesNothing(t *testing.T) {
	f := newSyncFixture(t)
	f.commitUpstream(t, "beta 2", map[string]string{"skills/beta/SKILL.md": "beta v2\n"})
	before := snapshot(t, filepath.Dir(f.gxSkill))

	out := f.runSync(t, "--merge")

	for _, want := range []string{"gx-beta: ", "+beta v2"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	assertSnapshotUnchanged(t, before, snapshot(t, filepath.Dir(f.gxSkill)))
}

func TestSyncLicense(t *testing.T) {
	setup := func(t *testing.T, license string) *syncFixture {
		f := newMergeFixture(t)
		writeFile(t, filepath.Join(f.gxSkill, "LICENSE"), "MIT\n")
		f.commitUpstream(t, "edit", map[string]string{
			"skills/alpha/a.md": "up\n2\n3\n4\n5\n6\n7\n", "LICENSE": license,
		})
		return f
	}

	t.Run("unchanged is left alone", func(t *testing.T) {
		f := setup(t, "MIT\n")

		out := f.runSync(t, "--merge")

		if strings.Contains(out, "license") {
			t.Errorf("no license report expected:\n%s", out)
		}
		if got := readGx(t, f, "LICENSE"); got != "MIT\n" {
			t.Errorf("LICENSE = %q", got)
		}
		if got := readGx(t, f, "a.md"); !strings.HasPrefix(got, "up\n") {
			t.Errorf("skill should still merge, a.md = %q", got)
		}
	})

	t.Run("changed is reported and nothing is written", func(t *testing.T) {
		f := setup(t, "MIT v2\n")
		before := snapshot(t, filepath.Dir(f.gxSkill))

		out := f.runSync(t, "--merge")

		for _, want := range []string{"gx-alpha: license changed", "-MIT", "+MIT v2"} {
			if !strings.Contains(out, want) {
				t.Errorf("report missing %q:\n%s", want, out)
			}
		}
		assertSnapshotUnchanged(t, before, snapshot(t, filepath.Dir(f.gxSkill)))
	})

	t.Run("--check reports a change with its diff and writes nothing", func(t *testing.T) {
		f := setup(t, "MIT v2\n")
		before := snapshot(t, filepath.Dir(f.gxSkill))

		out := f.runSync(t, "--check")

		for _, want := range []string{"gx-alpha: license changed", "-MIT", "+MIT v2", "gx-alpha: 1 new upstream commits"} {
			if !strings.Contains(out, want) {
				t.Errorf("report missing %q:\n%s", want, out)
			}
		}
		assertSnapshotUnchanged(t, before, snapshot(t, filepath.Dir(f.gxSkill)))
	})

	t.Run("--check with --accept-license still writes nothing", func(t *testing.T) {
		f := setup(t, "MIT v2\n")
		before := snapshot(t, filepath.Dir(f.gxSkill))

		out := f.runSync(t, "--check", "--accept-license", "gx-alpha")

		if !strings.Contains(out, "gx-alpha: license changed") {
			t.Errorf("report missing license change:\n%s", out)
		}
		assertSnapshotUnchanged(t, before, snapshot(t, filepath.Dir(f.gxSkill)))
	})

	t.Run("accepted change is copied and the skill updates", func(t *testing.T) {
		f := setup(t, "MIT v2\n")

		f.runSync(t, "--merge", "--accept-license", "gx-alpha")

		if got := readGx(t, f, "LICENSE"); got != "MIT v2\n" {
			t.Errorf("LICENSE = %q", got)
		}
		if got := readGx(t, f, "a.md"); !strings.HasPrefix(got, "up\n") {
			t.Errorf("a.md = %q", got)
		}
		if got := readGx(t, f, "README.md"); got != "readme v1\n" {
			t.Errorf("README.md must be untouched, got %q", got)
		}
	})
}

func TestSyncMissingUpstreamPath_IsReportedAndNothingIsWritten(t *testing.T) {
	f := newMergeFixture(t)
	gitIn(t, f.upstream, "rm", "-rq", "skills/alpha")
	gitIn(t, f.upstream, "commit", "-q", "-m", "drop alpha")
	before := snapshot(t, filepath.Dir(f.gxSkill))

	out := f.runSync(t, "--merge")

	if !strings.Contains(out, "gx-alpha: upstream path skills/alpha no longer exists") {
		t.Errorf("report:\n%s", out)
	}
	assertSnapshotUnchanged(t, before, snapshot(t, filepath.Dir(f.gxSkill)))
}

func TestRealManifest_SeedsTheBundledSkills(t *testing.T) {
	b, err := os.ReadFile("vendored-skills.manifest")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"gx-tdd mattpocock/skills skills/engineering/tdd LICENSE bd453a6 merge",
		"gx-to-tickets mattpocock/skills skills/engineering/to-tickets LICENSE 19c50d5 port",
		"gx-implement mattpocock/skills skills/engineering/implement LICENSE 386d4ff port",
	} {
		if !strings.Contains(strings.Join(strings.Fields(string(b)), " "), want) {
			t.Errorf("manifest missing %q", want)
		}
	}
}
