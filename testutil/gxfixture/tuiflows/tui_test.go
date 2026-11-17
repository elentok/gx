package tuiflows

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/vtterm"
)

const waitTimeout = 20 * time.Second

func TestMain(m *testing.M) { os.Exit(fixturetest.Run(m, nil)) }

// startGx runs the real gx binary in a terminal, wired to the fixture's server.
func startGx(t *testing.T, r *fixturetest.Running, args ...string) *vtterm.Term {
	t.Helper()
	cmd := exec.Command(fixturetest.GxBin, args...)
	cmd.Env = append(append(os.Environ(), r.Env...), "TERM=xterm-256color")
	cmd.Dir = r.Repo()
	return vtterm.Start(t, cmd, 140, 40)
}

func TestTUI_ServerModeShownAndDroppedOnStop(t *testing.T) {
	r := fixturetest.Start(t, "ready")
	v := startGx(t, r, "tickets")

	v.WaitText("ready-epic", waitTimeout)
	if _, _, ok := v.Find("server down"); ok {
		t.Fatalf("server down shown while the server runs:\n%v", v.Screen())
	}
	r.Gx(t, "server", "stop")
	v.WaitText("server down", waitTimeout)
}

func TestTUI_AgentsTerminalTypesShiftedLetters(t *testing.T) {
	r := fixturetest.Start(t, "ready")
	var res struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	out := r.Gx(t, "agents", "create", "--project", "demo", "--dir", r.Repo(), "--json")
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Session.ID == "" {
		t.Fatalf("agents create: %v\n%s", err, out)
	}
	t.Cleanup(func() { r.Gx(t, "agents", "stop", res.Session.ID) })

	v := startGx(t, r, "agents")
	v.WaitText("Welcome to Claude Code", waitTimeout)
	v.Send("\r") // focus the terminal
	v.WaitFor("the focused terminal's cursor", waitTimeout, func() bool {
		_, _, visible := v.Cursor()
		return visible
	})
	v.Send("Hi There!")
	v.WaitText("> Hi There!", waitTimeout)
}
