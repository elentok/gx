package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// RegisterProject points gx's data dir at a temp dir and registers repoRoot
// there as project "p" with the given trunk. It returns the project dir;
// rewrite its project.json to change trunk. Not parallel-safe (sets env).
func RegisterProject(t *testing.T, repoRoot, trunk string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	projectDir := filepath.Join(os.Getenv("XDG_DATA_HOME"), "gx", "tickets", "p")
	Mkdir(t, projectDir)
	WriteFile(t, projectDir, "project.json", `{"name":"p","repo":"`+repoRoot+`","trunk":"`+trunk+`"}`)
	return projectDir
}
