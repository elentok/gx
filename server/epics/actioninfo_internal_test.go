package epics

import (
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets/schema"
)

func TestActionInfo_ResumeLabelFollowsAnswer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		actions []string
		want    string
	}{
		{"beside answer", []string{wire.ActionAnswer, wire.ActionResume}, "Resume without answering"},
		{"alone", []string{wire.ActionResume}, schema.VerbResume.Label()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := actionInfo(c.actions)[wire.ActionResume].Label; got != c.want {
				t.Errorf("label = %q, want %q", got, c.want)
			}
		})
	}
}
