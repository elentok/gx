package ticketref

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/testutil"
)

// testDirs returns config dirs rooted in a fresh temp HOME and the ticket
// store inside it, so a test never touches the real one.
func testDirs(t *testing.T) (config.Dirs, string) {
	t.Helper()
	home := t.TempDir()
	env := []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
	}
	return config.DirsFrom(env), filepath.Join(home, ".local", "share", "gx", "tickets")
}

// addProject writes <store>/<name>/project.json pointing at repoPath and
// returns the project directory.
func addProject(t *testing.T, store, name, repoPath string) string {
	t.Helper()
	dir := filepath.Join(store, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{"name": name, "repo": repoPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestResolveEpicArg_BareNameAndFullPathResolveToSameEpic(t *testing.T) {
	dirs, store := testDirs(t)
	repoDir := testutil.TempRepo(t)
	project := addProject(t, store, "mine", repoDir)
	epicDir := filepath.Join(project, "widget-epic")
	testutil.Mkdir(t, epicDir)

	bareResolved := ResolveEpicArg(dirs, "widget-epic", repoDir)
	fullResolved := ResolveEpicArg(dirs, epicDir, repoDir)

	if bareResolved != epicDir {
		t.Errorf("ResolveEpicArg(bare name) = %q, want %q", bareResolved, epicDir)
	}
	if fullResolved != epicDir {
		t.Errorf("ResolveEpicArg(full path) = %q, want it returned unchanged", fullResolved)
	}
}

func TestResolveEpicArg_ProjectEpicAddressResolvesInCurrentAndOtherProject(t *testing.T) {
	dirs, store := testDirs(t)
	repoDir := testutil.TempRepo(t)
	otherRepoDir := testutil.TempRepo(t)
	mine := addProject(t, store, "mine", repoDir)
	other := addProject(t, store, "other", otherRepoDir)
	testutil.Mkdir(t, filepath.Join(mine, "widget-epic"))
	testutil.Mkdir(t, filepath.Join(other, "gadget-epic"))

	if got, want := ResolveEpicArg(dirs, "mine:widget-epic", repoDir), filepath.Join(mine, "widget-epic"); got != want {
		t.Errorf("ResolveEpicArg(current project address) = %q, want %q", got, want)
	}
	if got, want := ResolveEpicArg(dirs, "other:gadget-epic", repoDir), filepath.Join(other, "gadget-epic"); got != want {
		t.Errorf("ResolveEpicArg(other project address) = %q, want %q", got, want)
	}
}

func TestResolveEpicArg_UnknownBareNameReturnedUnchanged(t *testing.T) {
	dirs, store := testDirs(t)
	repoDir := testutil.TempRepo(t)
	addProject(t, store, "mine", repoDir)

	resolved := ResolveEpicArg(dirs, "no-such-epic", repoDir)

	if resolved != "no-such-epic" {
		t.Errorf("ResolveEpicArg(unknown name) = %q, want it returned unchanged", resolved)
	}
}
