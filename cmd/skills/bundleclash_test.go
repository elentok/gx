package skills_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

func requireBundleClash(t *testing.T, err error, name, source string) {
	t.Helper()
	if err == nil {
		t.Fatal("install with a bundle name clash exited zero")
	}
	real, _ := filepath.EvalSymlinks(source)
	for _, want := range []string{name, "skill bundle", real, "delete the old copy"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
}

func TestSkillsInstallSource_OldBundleCopySkipped(t *testing.T) {
	t.Parallel()
	for _, bundleFirst := range []bool{false, true} {
		name := "source first"
		if bundleFirst {
			name = "bundle first"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			d := fakeSkillsDeps(t, &stdout)
			claude, codex := agentRootsOf(t, d)
			src := clashSource(t, "gx-verify", "mine")
			if bundleFirst {
				if err := run(t, d, "install"); err != nil {
					t.Fatalf("bundle install: %v", err)
				}
			}
			err := run(t, d, "install", src)
			requireBundleClash(t, err, "gx-verify", src)
			if !bundleFirst {
				if err := run(t, d, "install"); err == nil {
					t.Fatal("bare install with a clash exited zero")
				}
			}
			for _, root := range []string{claude, codex} {
				if _, err := os.Readlink(filepath.Join(root, "gx-verify")); err == nil {
					t.Errorf("gx-verify under %s is still a source link", root)
				}
				if !exists(filepath.Join(root, "gx-verify", "SKILL.md")) {
					t.Errorf("bundle gx-verify missing under %s", root)
				}
				if !exists(filepath.Join(root, "mine")) {
					t.Errorf("mine missing under %s", root)
				}
			}
		})
	}
}

func TestSkillsInstall_LaterBundleNameRemovesSourceLink(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	d := fakeSkillsDeps(t, &stdout)
	claude, _ := agentRootsOf(t, d)
	src := clashSource(t, "newcomer", "mine")
	if err := run(t, d, "install", src); err != nil {
		t.Fatalf("source install: %v", err)
	}
	if !exists(filepath.Join(claude, "newcomer")) {
		t.Fatal("newcomer not linked")
	}

	d.BundleSkills = func() ([]string, error) { return []string{"gx-verify", "newcomer"}, nil }
	err := run(t, d, "install")
	requireBundleClash(t, err, "newcomer", src)
	if exists(filepath.Join(claude, "newcomer")) {
		t.Error("source link for newcomer survived the bundle taking the name")
	}
	if !exists(filepath.Join(claude, "mine")) {
		t.Error("mine was removed")
	}
}

func TestSkillsInstallDev_SourceNameOwnedByCheckoutIsDropped(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	repo := testutil.TempRepo(t)
	writeDevBundle(t, repo)
	d := fakeSkillsDevDeps(t, &stdout, repo)
	claude, _ := agentRootsOf(t, d)
	d.BundleSkills = func() ([]string, error) { return []string{"gx-implement", "newcomer"}, nil }
	src := clashSource(t, "newcomer", "mine")
	_ = run(t, d, "install", src)

	err := run(t, d, "install", "--dev")
	if err == nil || !strings.Contains(err.Error(), "dev checkout") {
		t.Fatalf("want a dev checkout clash error, got %v", err)
	}
	if exists(filepath.Join(claude, "newcomer")) {
		t.Error("source link for newcomer survived the dev install")
	}
	if !exists(filepath.Join(claude, "mine")) {
		t.Error("mine was removed")
	}
}
