package commit

import (
	"testing"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/testutil"
)

func TestReloadResolvesTrunk(t *testing.T) {
	repoDir := testutil.TempRepo(t)
	testutil.MustGitExported(t, repoDir, "branch", "develop")
	repo, err := git.FindRepo(repoDir)
	if err != nil {
		t.Fatalf("FindRepo: %v", err)
	}
	projectDir := testutil.RegisterProject(t, repo.Root, "develop")

	m := newTestModel(repoDir, "HEAD")
	if m.trunk != "develop" {
		t.Fatalf("trunk = %q, want develop", m.trunk)
	}

	testutil.WriteFile(t, projectDir, "project.json", `{"name":"p","repo":"`+repo.Root+`","trunk":"nope"}`)
	m, _ = m.WithRef("HEAD")
	if m.trunk != "" {
		t.Fatalf("trunk = %q after it went missing, want none", m.trunk)
	}
}
