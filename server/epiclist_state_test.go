package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
	"time"

	"github.com/elentok/gx/tickets"
)

// epicStateValues parses epicsummary.go for every EpicState constant, so a new
// state cannot slip past the guard below.
func epicStateValues(t *testing.T) []EpicState {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "epicsummary.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []EpicState
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "EpicState" {
				continue
			}
			for _, v := range vs.Values {
				s, err := strconv.Unquote(v.(*ast.BasicLit).Value)
				if err != nil {
					t.Fatal(err)
				}
				out = append(out, EpicState(s))
			}
		}
	}
	return out
}

func TestEpicListState_EveryEpicStateHasItsOwn(t *testing.T) {
	states := epicStateValues(t)
	if len(states) == 0 {
		t.Fatal("found no EpicState constants")
	}
	for _, s := range states {
		e := tickets.Epic{Name: "epic"}
		want := EpicListState(s)
		switch s {
		case EpicStateFinished:
			// Finished is decided by merged_at, not by the queue state.
			e.MergedAt = time.Unix(100, 0)
		case EpicStateWaiting:
			// A waiting epic is queued, ready or has no open ticket, depending
			// on the queue and its tickets; it has no list state of its own.
			continue
		}
		if got := epicListState(e, false, false, s); got != want {
			t.Errorf("epicListState(%q) = %q, want %q", s, got, want)
		}
	}
}
