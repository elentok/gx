package server

import (
	"testing"

	"github.com/elentok/gx/server/queue"
)

func TestRootRef_ParseAndStringRoundTrip(t *testing.T) {
	t.Parallel()
	root, err := queue.ParseRootRef("proj:epic-a")
	if err != nil {
		t.Fatal(err)
	}
	if root != (queue.RootRef{Project: "proj", Epic: "epic-a"}) || root.String() != "proj:epic-a" {
		t.Errorf("root = %+v, String = %q", root, root.String())
	}
}

func TestRootRef_RefusesInvalidInput(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "proj", ":epic", "proj:"} {
		if _, err := queue.ParseRootRef(in); err == nil {
			t.Errorf("parseRootRef(%q) accepted", in)
		}
	}
}
