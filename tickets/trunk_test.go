package tickets

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil"
)

// trunkFixture is a real repo registered as project "p" in a temp ticket
// store. Not parallel-safe (sets env).
func trunkFixture(t *testing.T, projectJSON string) (repo, projectDir string) {
	t.Helper()
	repo = testutil.TempRepo(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	projectDir = filepath.Join(os.Getenv("XDG_DATA_HOME"), "gx", "tickets", "p")
	testutil.Mkdir(t, projectDir)
	if projectJSON != "" {
		testutil.WriteFile(t, projectDir, "project.json", projectJSON)
	}
	return repo, projectDir
}

func TestErrTrunkMissing_RemoteOnlyKeepsOtherHints(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		err  ErrTrunkMissing
		want string
	}{
		"unregistered": {ErrTrunkMissing{Repo: "/r", Trunk: "dev", RemoteOnly: true}, `gx project add .`},
		"detected":     {ErrTrunkMissing{Project: "p", Repo: "/r", Trunk: "dev", RemoteOnly: true}, `set "trunk" in project.json`},
	}
	for name, tc := range cases {
		msg := tc.err.Error()
		for _, hint := range []string{"git branch dev origin/dev", tc.want} {
			if !strings.Contains(msg, hint) {
				t.Errorf("%s: %q lacks %q", name, msg, hint)
			}
		}
	}
}

func registered(repo, extra string) string {
	return `{"name":"p","repo":"` + repo + `"` + extra + `}`
}

func TestTrunk_SetAndExisting(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "branch", "develop")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"develop"`))

	got, err := Trunk(projectDir, repo)
	if err != nil || got != "develop" {
		t.Fatalf("Trunk() = %q, %v; want develop", got, err)
	}
}

func TestTrunk_UnsetUsesRemoteDefaultAndIsNotWrittenBack(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "branch", "develop")
	testutil.MustGitExported(t, repo, "update-ref", "refs/remotes/origin/develop", "HEAD")
	testutil.MustGitExported(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
	body := registered(repo, "")
	testutil.WriteFile(t, projectDir, "project.json", body)

	got, err := TrunkFor(repo)
	if err != nil || got != "develop" {
		t.Fatalf("TrunkFor() = %q, %v; want develop", got, err)
	}
	after, _ := os.ReadFile(filepath.Join(projectDir, "project.json"))
	if string(after) != body {
		t.Errorf("project.json rewritten: %s", after)
	}
}

func TestTrunk_ReadsProjectFileEveryCall(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "branch", "develop")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, ""))
	if got, _ := Trunk(projectDir, repo); got != "main" {
		t.Fatalf("Trunk() = %q, want main", got)
	}
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"develop"`))
	if got, _ := Trunk(projectDir, repo); got != "develop" {
		t.Fatalf("Trunk() after edit = %q, want develop", got)
	}
}

func TestTrunk_MissingBranch(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"develop"`))

	_, err := Trunk(projectDir, repo)
	var missing *ErrTrunkMissing
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want *ErrTrunkMissing", err)
	}
	if missing.Project != "p" || missing.Trunk != "develop" || !missing.FromProjectFile {
		t.Errorf("err fields = %+v", missing)
	}
	want := `project "p": trunk "develop" from project.json is not a local branch`
	if err.Error() != want {
		t.Errorf("err = %q\nwant  %q", err, want)
	}
}

func TestTrunk_MissingBranchWithRemoteHints(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "update-ref", "refs/remotes/origin/develop", "HEAD")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"develop"`))

	_, err := Trunk(projectDir, repo)
	want := `project "p": trunk "develop" from project.json is not a local branch; create it with "git branch develop origin/develop"`
	if err == nil || err.Error() != want {
		t.Errorf("err = %v\nwant  %q", err, want)
	}
}

func TestTrunk_DetectedMissingInRegisteredProject(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "branch", "-m", "main", "other")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, ""))

	_, err := Trunk(projectDir, repo)
	var missing *ErrTrunkMissing
	if !errors.As(err, &missing) || missing.FromProjectFile {
		t.Fatalf("err = %v, want detected *ErrTrunkMissing", err)
	}
	want := `project "p": detected trunk "main" is not a local branch; set "trunk" in project.json`
	if err.Error() != want {
		t.Errorf("err = %q\nwant  %q", err, want)
	}
}

func TestTrunk_MalformedValue(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"bad..name"`))

	_, err := Trunk(projectDir, repo)
	var missing *ErrTrunkMissing
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want *ErrTrunkMissing", err)
	}
	want := `project "p": trunk "bad..name" from project.json is not a valid branch name`
	if err.Error() != want {
		t.Errorf("err = %q\nwant  %q", err, want)
	}
}

func TestTrunk_UnreadableProjectFile(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.WriteFile(t, projectDir, "project.json", `{"name":`)

	_, err := Trunk(projectDir, repo)
	var missing *ErrTrunkMissing
	if err == nil || errors.As(err, &missing) {
		t.Fatalf("err = %v, want a read error, not ErrTrunkMissing", err)
	}
}

func TestTrunkFor_UnregisteredRepoUsesDetectedDefault(t *testing.T) {
	repo, _ := trunkFixture(t, "")

	got, err := TrunkFor(repo)
	if err != nil || got != "main" {
		t.Fatalf("TrunkFor() = %q, %v; want main", got, err)
	}
}

func TestTrunkFor_UnregisteredRepoMissingNamesRepo(t *testing.T) {
	repo, _ := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "branch", "-m", "main", "other")

	_, err := TrunkFor(repo)
	var missing *ErrTrunkMissing
	if !errors.As(err, &missing) || missing.Project != "" {
		t.Fatalf("err = %v, want unregistered *ErrTrunkMissing", err)
	}
	want := `repo ` + repo + `: detected trunk "main" is not a local branch; run "gx project add ." then set "trunk" in its project.json`
	if err.Error() != want {
		t.Errorf("err = %q\nwant  %q", err, want)
	}
}

func TestOpenRepo_SetsTrunk(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.MustGitExported(t, repo, "branch", "develop")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"develop"`))

	r, err := OpenRepo(repo)
	if err != nil || r.Trunk != "develop" {
		t.Fatalf("OpenRepo() = %+v, %v; want Trunk develop", r, err)
	}
}

func TestOpenRepo_ResolverErrorKeepsUsableRepo(t *testing.T) {
	repo, projectDir := trunkFixture(t, "")
	testutil.WriteFile(t, projectDir, "project.json", registered(repo, `,"trunk":"develop"`))

	r, err := OpenRepo(repo)
	var missing *ErrTrunkMissing
	if !errors.As(err, &missing) {
		t.Fatalf("err = %v, want *ErrTrunkMissing", err)
	}
	if r == nil || r.Root != repo || r.Trunk != "" {
		t.Errorf("OpenRepo() repo = %+v, want Root %s and empty Trunk", r, repo)
	}
}

func TestOpenRepo_NotARepo(t *testing.T) {
	r, err := OpenRepo(t.TempDir())
	if err == nil || r != nil {
		t.Errorf("OpenRepo() = %+v, %v; want nil repo and an error", r, err)
	}
}

func TestTrunkCheckout(t *testing.T) {
	t.Run("normal repo is its root", func(t *testing.T) {
		repo, projectDir := trunkFixture(t, "")
		testutil.WriteFile(t, projectDir, "project.json", registered(repo, ""))
		if got := TrunkCheckout(filepath.Dir(projectDir), "p"); got != repo {
			t.Errorf("TrunkCheckout() = %q, want %q", got, repo)
		}
	})

	bare := func(t *testing.T) (bareDir, projectDir string) {
		repo, projectDir := trunkFixture(t, "")
		bareDir = filepath.Join(t.TempDir(), "r.git")
		testutil.MustGitExported(t, repo, "clone", "--bare", repo, bareDir)
		testutil.WriteFile(t, projectDir, "project.json", registered(bareDir, `,"trunk":"main"`))
		return bareDir, projectDir
	}

	t.Run("bare repo is the worktree on trunk", func(t *testing.T) {
		bareDir, projectDir := bare(t)
		wt := filepath.Join(t.TempDir(), "main-wt")
		testutil.MustGitExported(t, bareDir, "worktree", "add", wt, "main")
		got := TrunkCheckout(filepath.Dir(projectDir), "p")
		if canonicalPath(got) != canonicalPath(wt) {
			t.Errorf("TrunkCheckout() = %q, want %q", got, wt)
		}
	})

	t.Run("bare repo without a trunk worktree is empty", func(t *testing.T) {
		_, projectDir := bare(t)
		if got := TrunkCheckout(filepath.Dir(projectDir), "p"); got != "" {
			t.Errorf("TrunkCheckout() = %q, want empty", got)
		}
	})

	t.Run("missing trunk is empty", func(t *testing.T) {
		_, projectDir := bare(t)
		testutil.WriteFile(t, projectDir, "project.json", registered(filepath.Join(filepath.Dir(projectDir), "x"), `,"trunk":"nope"`))
		if got := TrunkCheckout(filepath.Dir(projectDir), "p"); got != "" {
			t.Errorf("TrunkCheckout() = %q, want empty", got)
		}
	})

	t.Run("unknown project is empty", func(t *testing.T) {
		_, projectDir := trunkFixture(t, "")
		if got := TrunkCheckout(filepath.Dir(projectDir), "nope"); got != "" {
			t.Errorf("TrunkCheckout() = %q, want empty", got)
		}
	})
}
