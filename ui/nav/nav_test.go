package nav_test

import (
	"testing"

	"github.com/elentok/gx/ui/nav"
)

func TestPushAndIsPush(t *testing.T) {
	route := nav.Route{Tab: nav.TabLog, Ref: "HEAD"}
	cmd := nav.Open(route)
	msg := cmd()
	got, ok := nav.IsOpen(msg)
	if !ok {
		t.Fatal("expected IsPush=true")
	}
	if got.Tab != nav.TabLog || got.Ref != "HEAD" {
		t.Errorf("IsPush route = %+v, want {TabLog, HEAD}", got)
	}
}

func TestReplaceAndIsReplace(t *testing.T) {
	route := nav.Route{Tab: nav.TabStatus}
	cmd := nav.Switch(route)
	msg := cmd()
	got, ok := nav.IsSwitch(msg)
	if !ok {
		t.Fatal("expected IsReplace=true")
	}
	if got.Tab != nav.TabStatus {
		t.Errorf("IsReplace route kind = %q, want %q", got.Tab, nav.TabStatus)
	}
}

func TestBackAndIsBack(t *testing.T) {
	cmd := nav.Back()
	msg := cmd()
	if !nav.IsBack(msg) {
		t.Fatal("expected IsBack=true")
	}
}

func TestIsPush_WrongType(t *testing.T) {
	_, ok := nav.IsOpen("not-a-push")
	if ok {
		t.Error("expected IsPush=false for wrong type")
	}
}

func TestIsReplace_WrongType(t *testing.T) {
	_, ok := nav.IsSwitch(42)
	if ok {
		t.Error("expected IsReplace=false for wrong type")
	}
}

func TestIsBack_WrongType(t *testing.T) {
	if nav.IsBack("not-a-back") {
		t.Error("expected IsBack=false for wrong type")
	}
}
