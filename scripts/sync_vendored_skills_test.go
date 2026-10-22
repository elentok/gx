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
	cmd := exec.Command("bash", append([]string{"sync-vendored-skills.sh", "--manifest", f.manifest}, args...)...)
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
