package servertest

import (
	"os"
	"path/filepath"
	"testing"
)

// SetProjectRepo points the project at the repo its agents run in. Call it
// after WriteTicket, before Start. It opts the project in to auto-ff-merge so a
// finished epic merges; SetProjectRepoNoAutoMerge leaves the default (off).
func SetProjectRepo(t *testing.T, store, project, repo string) {
	t.Helper()
	writeProjectRepo(t, store, project, repo, `,"auto-ff-merge":true`)
}

// SetProjectTrunk is SetProjectRepo with project.json's trunk set. Trunk is
// read live, so it may also run mid-run.
func SetProjectTrunk(t *testing.T, store, project, repo, trunk string) {
	t.Helper()
	writeProjectRepo(t, store, project, repo, `,"auto-ff-merge":true,"trunk":"`+trunk+`"`)
}

// SetProjectTrunkNoAutoMerge is SetProjectTrunk without the auto-ff-merge opt-in.
func SetProjectTrunkNoAutoMerge(t *testing.T, store, project, repo, trunk string) {
	t.Helper()
	writeProjectRepo(t, store, project, repo, `,"trunk":"`+trunk+`"`)
}

// SetProjectRepoNoAutoMerge is SetProjectRepo without the auto-ff-merge opt-in.
func SetProjectRepoNoAutoMerge(t *testing.T, store, project, repo string) {
	t.Helper()
	writeProjectRepo(t, store, project, repo, "")
}

func writeProjectRepo(t *testing.T, store, project, repo, extra string) {
	t.Helper()
	if err := WriteProjectRepo(store, project, repo, extra); err != nil {
		t.Fatal(err)
	}
}

// WriteProjectRepo is SetProjectRepo for a caller with no *testing.T; extra is
// raw JSON members appended to project.json, such as `,"auto-ff-merge":true`.
func WriteProjectRepo(store, project, repo, extra string) error {
	body := `{"name":"` + project + `","repo":"` + repo + `"` + extra + `}`
	return os.WriteFile(filepath.Join(store, project, "project.json"), []byte(body), 0o644)
}
