package nav_test

import (
	"testing"

	"github.com/elentok/gx/ui/nav"
)

func TestOpenAndIsOpen(t *testing.T) {
	route := nav.ViewState{Tab: nav.TabLog, Ref: "HEAD"}
	cmd := nav.Open(route)
	msg := cmd()
	got, ok := nav.IsOpen(msg)
	if !ok {
		t.Fatal("expected IsOpen=true")
	}
	if got.Tab != nav.TabLog || got.Ref != "HEAD" {
		t.Errorf("IsOpen route = %+v, want {TabLog, HEAD}", got)
	}
}

func TestSwitchAndIsSwitch(t *testing.T) {
	route := nav.ViewState{Tab: nav.TabStatus}
	cmd := nav.Switch(route)
	msg := cmd()
	got, ok := nav.IsSwitch(msg)
	if !ok {
		t.Fatal("expected IsSwitch=true")
	}
	if got.Tab != nav.TabStatus {
		t.Errorf("IsSwitch route kind = %q, want %q", got.Tab, nav.TabStatus)
	}
}

func TestBackAndIsBack(t *testing.T) {
	cmd := nav.Back()
	msg := cmd()
	if !nav.IsBack(msg) {
		t.Fatal("expected IsBack=true")
	}
}

func TestIsOpen_WrongType(t *testing.T) {
	_, ok := nav.IsOpen("not-an-open")
	if ok {
		t.Error("expected IsOpen=false for wrong type")
	}
}

func TestIsSwitch_WrongType(t *testing.T) {
	_, ok := nav.IsSwitch(42)
	if ok {
		t.Error("expected IsSwitch=false for wrong type")
	}
}

func TestIsBack_WrongType(t *testing.T) {
	if nav.IsBack("not-a-back") {
		t.Error("expected IsBack=false for wrong type")
	}
}
