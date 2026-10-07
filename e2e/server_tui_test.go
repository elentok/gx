package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrctl"
)

// TestServerTUI_ShowsServerModeAndDropsOnStop boots the real TUI with
// orchestrator = "server" against a running server: the tickets come from the
// server and the indicator names its pid, then goes down when the server stops.
func TestServerTUI_ShowsServerModeAndDropsOnStop(t *testing.T) {
	herdrctl.RequireHerdr(t)

	root, err := os.MkdirTemp("", "gxt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	home := filepath.Join(root, "home")
	e := &lifecycleEnv{t: t, bin: gxBinary(t), env: []string{
		"XDG_DATA_HOME=" + filepath.Join(root, "data"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"HOME=" + home,
	}}
	configDir := filepath.Join(home, ".config", "gx")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"orchestrator":"server"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store := filepath.Join(root, "data", "gx", "tickets")
	const project, epic = "tuiproj", "visible-epic"
	repo := testutil.TempRepo(t)
	servertest.WriteTicket(t, store, project, epic, "01", "tiny", "")
	servertest.SetProjectRepo(t, store, project, repo)

	e.gx("server", "start")
	t.Cleanup(func() {
		if _, up := e.running(); up {
			e.gx("server", "stop")
		}
	})
	h, up := e.running()
	if !up {
		t.Fatal("server not running after `gx server start`")
	}

	ws := herdrctl.NewWorkspace(t, repo)
	ws.Run(append(append([]string{"env"}, e.env...), e.bin, "tickets")...)
	ws.WaitForText(fmt.Sprintf("server ● pid %d", h.Pid), 30*time.Second)
	ws.WaitForText(epic, 15*time.Second)

	e.gx("server", "stop")
	ws.WaitForText("server ○ down", 15*time.Second)
}
