package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/servertest"
)

// codexSession seeds a Codex session record: its "/" list is the built-ins,
// so no harness starts.
func codexSession(t *testing.T, h *servertest.Harness) string {
	t.Helper()
	dir := filepath.Join(h.SessionsDir, "cdx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]any{"id": "cdx", "harness": "codex", "project": "proj", "dir": t.TempDir()})
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	return "cdx"
}

func TestAgentsCommands_PrintsRows(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir())
	id := codexSession(t, h)
	var out bytes.Buffer
	if err := runAgentsCommands(context.Background(), h.Client, &out, false, id, "", "", "/"); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, line := range strings.Split(out.String(), "\n") {
		if cols := strings.Split(line, "\t"); len(cols) == 4 && cols[0] == "compact" && cols[2] == "built-in" {
			found = true
		}
	}
	if !found {
		t.Errorf("no compact row in %q", out.String())
	}
}

func TestAgentsCommands_JSON(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir())
	id := codexSession(t, h)
	var out bytes.Buffer
	if err := runAgentsCommands(context.Background(), h.Client, &out, true, id, "", "", "/"); err != nil {
		t.Fatal(err)
	}
	var got wire.SessionCommandsResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || len(got.Entries) == 0 || got.Degraded {
		t.Errorf("output %q: %+v, %v; want the built-ins", out.String(), got, err)
	}
}

func TestAgentsCommands_RefusalIsAnError(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir())
	var out bytes.Buffer
	err := runAgentsCommands(context.Background(), h.Client, &out, false, "nope", "", "", "/")
	if err == nil || !strings.Contains(err.Error(), wire.ReasonSessionUnknown) {
		t.Errorf("err = %v, want it to name %s", err, wire.ReasonSessionUnknown)
	}
}
