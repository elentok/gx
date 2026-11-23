package tuiflows

import (
	"encoding/json"
	"image/color"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/catalog"
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
	v.WaitText("Hi There!", waitTimeout)
}

// TestTUI_NotSentRowIsYellowWithNoActionAndCopyKey: a send that may not have landed shows the yellow row with the
// text as sent, the Action key answers "no action" and the copy key copies the card text.
func TestTUI_NotSentRowIsYellowWithNoActionAndCopyKey(t *testing.T) {
	r := fixturetest.Start(t, "deliver-cards")
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil || len(list) != len(catalog.DeliverCardChats) {
		t.Fatalf("sessions = %d, %v", len(list), err)
	}
	var id, conv string
	for _, s := range list {
		var c wire.SessionChatResponse
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", s.ID, "--json")), &c); err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(c.Messages, func(m wire.SessionChatMessage) bool { return strings.Contains(m.Text, catalog.DeliverCardChats[0]) }) {
			id, conv = s.ID, c.ConversationID
		} else {
			// Only the not-sent chat stays, so the tab's selected row is the one with the card.
			r.Gx(t, "agents", "remove", "--yes", s.ID)
		}
	}
	if id == "" {
		t.Fatal("no not-sent chat")
	}
	if err := agentfake.SendControl(agentfake.TUIControlPath(r.Home(), conv), agentfake.Control{Action: agentfake.ActionUndecided}); err != nil {
		t.Fatal(err)
	}
	r.Gx(t, "agents", "prompt", id, "/model opus")
	if !testutil.Poll(30*time.Second, 200*time.Millisecond, func() bool {
		return strings.Contains(r.Gx(t, "agents", "list", "--json"), wire.DeliveryMaybeSent)
	}) {
		t.Fatal("the chat never raised the not-sent card")
	}

	v := startGx(t, r, "agents")
	x, y := v.WaitText("May not have been sent: /model opus", waitTimeout)
	v.WaitText("no resend", waitTimeout)
	if got, want := rgb(v.FG(x, y)), [3]uint32{0xf9, 0xe2, 0xaf}; got != want {
		t.Fatalf("row colour = %v, want the yellow %v", got, want)
	}
	v.Send("\r") // focus the terminal
	const leader = "\x01"
	v.Send(leader + "a")
	v.WaitText("no action", waitTimeout)
	v.Send(leader + "y")
	v.WaitText("copied", waitTimeout)
}

func rgb(c color.Color) [3]uint32 {
	if c == nil {
		return [3]uint32{}
	}
	r, g, b, _ := c.RGBA()
	return [3]uint32{r >> 8, g >> 8, b >> 8}
}
