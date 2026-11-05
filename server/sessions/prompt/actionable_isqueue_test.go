package prompt_test

import (
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestSessionActionable_IsQueue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a    wire.SessionActionable
		want bool
	}{
		{"queue with target", wire.SessionActionable{Kind: wire.SessionActionableQueue, Target: "gx:epic-impl"}, true},
		{"queue without target", wire.SessionActionable{Kind: wire.SessionActionableQueue}, false},
		{"prompt action", wire.SessionActionable{Reason: "done", Action: []string{"/next"}}, false},
		{"derived blocked", wire.SessionActionable{Kind: "blocked", Reason: "blocked"}, false},
	}
	for _, c := range cases {
		if got := c.a.IsQueue(); got != c.want {
			t.Errorf("%s: IsQueue() = %v, want %v", c.name, got, c.want)
		}
	}
}
