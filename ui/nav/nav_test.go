package nav_test

import (
	"testing"

	"github.com/elentok/gx/ui/nav"
)

func TestPushAndIsPush(t *testing.T) {
	route := nav.Route{Kind: nav.RouteLog, Ref: "HEAD"}
	cmd := nav.Push(route)
	msg := cmd()
	got, ok := nav.IsPush(msg)
	if !ok {
		t.Fatal("expected IsPush=true")
	}
	if got.Kind != nav.RouteLog || got.Ref != "HEAD" {
		t.Errorf("IsPush route = %+v, want {RouteLog, HEAD}", got)
	}
}

func TestReplaceAndIsReplace(t *testing.T) {
	route := nav.Route{Kind: nav.RouteStatus}
	cmd := nav.Replace(route)
	msg := cmd()
	got, ok := nav.IsReplace(msg)
	if !ok {
		t.Fatal("expected IsReplace=true")
	}
	if got.Kind != nav.RouteStatus {
		t.Errorf("IsReplace route kind = %q, want %q", got.Kind, nav.RouteStatus)
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
	_, ok := nav.IsPush("not-a-push")
	if ok {
		t.Error("expected IsPush=false for wrong type")
	}
}

func TestIsReplace_WrongType(t *testing.T) {
	_, ok := nav.IsReplace(42)
	if ok {
		t.Error("expected IsReplace=false for wrong type")
	}
}

func TestIsBack_WrongType(t *testing.T) {
	if nav.IsBack("not-a-back") {
		t.Error("expected IsBack=false for wrong type")
	}
}
