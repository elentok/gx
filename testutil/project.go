package testutil

import (
	"path/filepath"
	"testing"
)

// RegisterProject registers repoRoot as project "p" with the given trunk in a
// temp ticket store. It returns the project dir and the store path to put in
// the model's settings; rewrite the project dir's project.json to change trunk.
func RegisterProject(t *testing.T, repoRoot, trunk string) (projectDir, store string) {
	t.Helper()
	store = t.TempDir()
	projectDir = filepath.Join(store, "p")
	Mkdir(t, projectDir)
	WriteFile(t, projectDir, "project.json", `{"name":"p","repo":"`+repoRoot+`","trunk":"`+trunk+`"}`)
	return projectDir, store
}
