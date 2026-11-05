package serverctl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/health"
)

const (
	startFailureFile = "server-start-failure.log"
	// startFailureStderrLines is how much of server.stderr the report keeps.
	startFailureStderrLines = 20
)

// saveStartFailure appends what the supervisor knows about a server that never
// answered to server-start-failure.log in stateDir and returns its path, or ""
// when nothing could be written. The server's own log is no help here: a server
// that never started wrote nothing.
func saveStartFailure(stateDir string, svc serviceManager) string {
	path := filepath.Join(stateDir, startFailureFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return ""
	}
	defer f.Close()
	status := svc.Status()
	saveStartIncident(stateDir, status)
	fmt.Fprintf(f, "=== %s: server did not answer ===\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "--- supervisor state ---\n%s\n", strings.TrimSpace(status))
	fmt.Fprintf(f, "--- last %d lines of server.stderr ---\n%s\n\n",
		startFailureStderrLines, tailLines(filepath.Join(stateDir, "server.stderr"), startFailureStderrLines))
	return path
}

// saveStartIncident leaves the note the next server turns into a ticket. It
// overwrites: one open incident is enough.
func saveStartIncident(stateDir, status string) {
	note := fmt.Sprintf("Time: %s\n\n%s\n", time.Now().Format(time.RFC3339), strings.TrimSpace(status))
	_ = os.MkdirAll(stateDir, 0o700)
	_ = os.WriteFile(health.StartIncidentPath(stateDir), []byte(note), 0o600)
}

func tailLines(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(unreadable: " + err.Error() + ")"
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
