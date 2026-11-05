package serverctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type statusOnlyService struct{ serviceManager }

func (statusOnlyService) Status() string { return "state = spawn failed\nlast exit code = 78" }

func TestSaveStartFailure_ReportsSupervisorStateAndStderrTail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "server.stderr"), []byte("boom\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path := saveStartFailure(dir, statusOnlyService{})

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"spawn failed", "last exit code = 78", "boom"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("report missing %q:\n%s", want, got)
		}
	}
}
