package server_test

import (
	"testing"

	"github.com/elentok/gx/server"
)

func TestSessionActionable_IsQueue(t *testing.T) {
	cases := []struct {
		name string
		a    server.SessionActionable
		want bool
	}{
		{"queue with target", server.SessionActionable{Kind: server.SessionActionableQueue, Target: "gx:epic-impl"}, true},
		{"queue without target", server.SessionActionable{Kind: server.SessionActionableQueue}, false},
		{"prompt action", server.SessionActionable{Reason: "done", Action: []string{"/next"}}, false},
		{"derived blocked", server.SessionActionable{Kind: "blocked", Reason: "blocked"}, false},
	}
	for _, c := range cases {
		if got := c.a.IsQueue(); got != c.want {
			t.Errorf("%s: IsQueue() = %v, want %v", c.name, got, c.want)
		}
	}
}
