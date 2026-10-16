package agents

import (
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/viewmodel"
)

func TestAFK_LeaderZToggles(t *testing.T) {
	f := &fakeSessionClient{}
	_, cmd := leaderThen(t, actionModel(t, f, "live"), letter("z"))
	if cmd == nil {
		t.Fatal("leader z started nothing")
	}
	cmd()
	if got := calls(f); !slices.Equal(got, []string{"afk toggle"}) {
		t.Fatalf("calls = %v, want an AFK toggle", got)
	}
}

func TestAFK_HeaderMarksAFK(t *testing.T) {
	f := &fakeSessionClient{}
	m := actionModel(t, f, "live")
	if strings.Contains(plainView(m), "AFK") {
		t.Fatalf("AFK shown while off:\n%s", plainView(m))
	}
	next, _ := m.WithServerState(&viewmodel.State{AFK: true, Sessions: actionSessions()}, m.link)
	if view := plainView(next.(Model)); !strings.Contains(view, "AFK") {
		t.Fatalf("no AFK mark while on:\n%s", view)
	}
}
