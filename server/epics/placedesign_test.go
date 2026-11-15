package epics

import (
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
)

func TestPlaceDesign_DelegatingShowsDelegating(t *testing.T) {
	p := placeDesign("proj:map", designActivity{running: true, delegating: true}, false, time.Time{})
	if p.section != wire.SectionDesigning || p.statusWords != StatusDelegating {
		t.Fatalf("got section %q words %q", p.section, p.statusWords)
	}
}
