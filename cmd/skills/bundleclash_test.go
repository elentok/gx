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

func TestSkillsInstall_BundleNameLandsSameRunAsSourceLinkDrop(t *testing.T) {
	t.Parallel()
	var outcomes []string
	for _, args := range [][]string{{"install"}, {"install", "--force-all"}} {
		var stdout bytes.Buffer
		d := fakeSkillsDeps(t, &stdout)
		claude, _ := agentRootsOf(t, d)
		src := clashSource(t, "gx-verify", "mine")
		d.BundleSkills = func() ([]string, error) { return nil, nil }
		if err := run(t, d, "install", src); err != nil {
			t.Fatalf("source install: %v", err)
		}
		d.BundleSkills = nil

		err := run(t, d, args...)
		requireBundleClash(t, err, "gx-verify", src)
		if strings.Contains(err.Error(), "conflict") {
			t.Errorf("%v: bundle install aborted as conflicted: %v", args, err)
		}
		if _, lerr := os.Readlink(filepath.Join(claude, "gx-verify")); lerr == nil {
			t.Errorf("%v: gx-verify is still a source link", args)
		}
		if !exists(filepath.Join(claude, "gx-verify", "SKILL.md")) {
			t.Errorf("%v: bundle gx-verify missing after one run", args)
		}
		outcomes = append(outcomes, strings.ReplaceAll(err.Error(), src, "SRC"))
	}
	if len(outcomes) == 2 && outcomes[0] != outcomes[1] {
		t.Errorf("--force-all changed the outcome:\n%s\n---\n%s", outcomes[0], outcomes[1])
	}
}

func TestSkillsInstallDev_CheckoutNamesLandSameRunAsSourceLinkDrop(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	repo := testutil.TempRepo(t)
	writeDevBundle(t, repo)
	writeFile(t, filepath.Join(repo, "skills", "newcomer", "SKILL.md"), "newcomer")
	d := fakeSkillsDevDeps(t, &stdout, repo)
	claude, _ := agentRootsOf(t, d)
	src := clashSource(t, "gx-verify", "newcomer", "mine")
	d.BundleSkills = func() ([]string, error) { return nil, nil }
	if err := run(t, d, "install", src); err != nil {
		t.Fatalf("source install: %v", err)
	}
	d.BundleSkills = nil

	err := run(t, d, "install", "--dev")
	if err == nil || !strings.Contains(err.Error(), "dev checkout") || strings.Contains(err.Error(), "conflict") {
		t.Fatalf("want only a dev checkout clash error, got %v", err)
	}
	if !exists(filepath.Join(claude, "gx-verify", "SKILL.md")) {
		t.Error("checkout gx-verify missing after one run")
	}
	if target, _ := os.Readlink(filepath.Join(claude, "gx-verify")); strings.Contains(target, "src") {
		t.Errorf("gx-verify still links the source: %s", target)
	}
	if exists(filepath.Join(claude, "newcomer")) {
		t.Error("source link for the checkout-only newcomer survived")
	}
	if !exists(filepath.Join(claude, "mine")) {
		t.Error("mine was removed")
	}
}
