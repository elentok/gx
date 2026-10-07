package server

import "testing"

func TestRootRef_ParseAndStringRoundTrip(t *testing.T) {
	root, err := parseRootRef("proj:epic-a")
	if err != nil {
		t.Fatal(err)
	}
	if root != (rootRef{Project: "proj", Epic: "epic-a"}) || root.String() != "proj:epic-a" {
		t.Errorf("root = %+v, String = %q", root, root.String())
	}
}

func TestRootRef_RefusesInvalidInput(t *testing.T) {
	for _, in := range []string{"", "proj", ":epic", "proj:"} {
		if _, err := parseRootRef(in); err == nil {
			t.Errorf("parseRootRef(%q) accepted", in)
		}
	}
}
