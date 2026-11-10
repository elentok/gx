package skills_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	skillscmd "github.com/elentok/gx/cmd/skills"
	"github.com/elentok/gx/skills"
)

// treeDigest hashes every path, link target and file body under dir.
func treeDigest(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fmt.Fprintln(h, path)
		if e.Type()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			fmt.Fprintln(h, target)
			return err
		}
		if e.Type().IsRegular() {
			body, err := os.ReadFile(path)
			h.Write(body)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// aliasClaudeRootToSource turns the claude root into a link to src, like a
// ~/.claude/skills symlinked to a dotfiles folder, and puts an old bundle copy
// in src.
func aliasClaudeRootToSource(t *testing.T, d skillscmd.Deps, src string) {
	t.Helper()
	claude, _ := agentRootsOf(t, d)
	writeFile(t, filepath.Join(src, "gx-old", "SKILL.md"), "old bundle copy")
	if err := os.RemoveAll(claude); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, claude); err != nil {
		t.Fatal(err)
	}
}

func assertOverlapSteps(t *testing.T, msg string) {
	t.Helper()
	for _, want := range []string{"resolves into the skill source", "remove the root link", "make the root a real folder", "delete the old gx-* bundle copies", "rerun"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q: %s", want, msg)
		}
	}
}

func TestSkillsInstallSource_RefusesRootAliasedToSource(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	src := writeSource(t)
	aliasClaudeRootToSource(t, d, src)
	before := treeDigest(t, src)

	err := run(t, d, "install", src)
	if err == nil {
		t.Fatal("install through an aliased root succeeded")
	}
	assertOverlapSteps(t, err.Error())
	if after := treeDigest(t, src); after != before {
		t.Error("source changed")
	}
}

func TestSkillsInstallBare_RefusesRootAliasedToSource(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	src := writeSource(t)
	if err := run(t, d, "install", src); err != nil {
		t.Fatal(err)
	}
	aliasClaudeRootToSource(t, d, src)
	before := treeDigest(t, src)

	err := run(t, d, "install")
	if err == nil {
		t.Fatal("bare install through an aliased root succeeded")
	}
	assertOverlapSteps(t, err.Error())
	if after := treeDigest(t, src); after != before {
		t.Error("source changed")
	}
	assertOverlapSteps(t, skills.ReadProblem(statusPathOf(t, d)))
}

func TestRefresh_RootAliasedToSourceRecordsProblem(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	src := writeSource(t)
	if err := run(t, d, "install", src); err != nil {
		t.Fatal(err)
	}
	aliasClaudeRootToSource(t, d, src)
	before := treeDigest(t, src)

	skillscmd.Refresh(d, &stdout)

	assertOverlapSteps(t, skills.ReadProblem(statusPathOf(t, d)))
	if after := treeDigest(t, src); after != before {
		t.Error("source changed")
	}
}
