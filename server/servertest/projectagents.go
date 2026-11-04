package servertest

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// WriteProjectAgents rewrites <store>/proj/project.json for repo with the
// extra JSON fields in agents (a leading-comma fragment such as `,"agents":{...}`).
func WriteProjectAgents(t *testing.T, store, repo, agents string) {
	t.Helper()
	body := `{"name":"proj","repo":"` + repo + `","auto-ff-merge":true` + agents + `}`
	if err := os.WriteFile(filepath.Join(store, "proj", "project.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// HasFlag reports whether args holds flag immediately followed by value.
func HasFlag(args []string, flag, value string) bool {
	i := slices.Index(args, flag)
	return i >= 0 && i+1 < len(args) && args[i+1] == value
}
