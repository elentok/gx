package skills_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/skills"
)

func clashSource(t *testing.T, names ...string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "src")
	for _, name := range names {
		writeFile(t, filepath.Join(src, name, "SKILL.md"), name+" from "+src)
	}
	return src
}

func TestSkillsInstallSource_NameClashSkippedInBothSources(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		firstCopy    bool
		secondCopy   bool
		reverseOrder bool
		force        string
	}{
		{"link-link", false, false, false, ""},
		{"link-copy", false, true, false, ""},
		{"copy-link", true, false, false, ""},
		{"copy-copy", true, true, false, ""},
		{"link-link reversed", false, false, true, ""},
		{"link-copy reversed", false, true, true, ""},
		{"link-link force-all", false, false, false, "--force-all"},
		{"link-copy reversed force-all", false, true, true, "--force-all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			d := fakeSkillsDeps(t, &stdout)
			claude, codex := agentRootsOf(t, d)
			a := clashSource(t, "shared", "only-a")
			b := clashSource(t, "shared", "only-b")
			args := func(src string, copyFiles bool) []string {
				cmd := []string{"install"}
				if tc.force != "" {
					cmd = append(cmd, tc.force)
				}
				if copyFiles {
					cmd = append(cmd, "--copy")
				}
				return append(cmd, src)
			}
			first, second := args(a, tc.firstCopy), args(b, tc.secondCopy)
			if tc.reverseOrder {
				first, second = second, first
			}

			if err := run(t, d, first...); err != nil {
				t.Fatalf("first install: %v", err)
			}
			err := run(t, d, second...)
			if err == nil {
				t.Fatal("clashing install exited zero")
			}
			realA, _ := filepath.EvalSymlinks(a)
			realB, _ := filepath.EvalSymlinks(b)
			for _, want := range []string{"shared", realA, realB} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error lacks %q: %v", want, err)
				}
			}
			for _, root := range []string{claude, codex} {
				if exists(filepath.Join(root, "shared", "SKILL.md")) {
					t.Errorf("shared still installed under %s", root)
				}
				for _, other := range []string{"only-a", "only-b"} {
					if !exists(filepath.Join(root, other)) {
						t.Errorf("%s missing under %s", other, root)
					}
				}
			}
		})
	}
}

func TestSkillsInstallSource_NameClashIsRecordedOnRefresh(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	a, b := clashSource(t, "shared"), clashSource(t, "shared")
	_ = run(t, d, "install", a)
	_ = run(t, d, "install", b)

	err := run(t, d, "install")
	if err == nil {
		t.Fatal("bare install with a clash exited zero")
	}
	if strings.Count(err.Error(), `"shared"`) != 1 {
		t.Errorf("clash should be reported once: %v", err)
	}
	if problem := skills.ReadProblem(statusPathOf(t, d)); !strings.Contains(problem, "shared") {
		t.Errorf("recorded problem lacks the clash: %q", problem)
	}
}
