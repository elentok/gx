package server

import (
	"reflect"
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestChangedEpicMeta(t *testing.T) {
	scan := func(step string, loop tickets.Epic) map[string][]tickets.Epic {
		loop.Path = "/store/p/loop"
		return map[string][]tickets.Epic{
			"/store/p": {{Path: "/store/p/design", DesignStep: step}, loop},
		}
	}
	none := tickets.Epic{}
	blocked := tickets.Epic{MergeBlockKind: "dirty-trunk", MergeBlockReason: "trunk is dirty", MergeBlockFiles: []string{"a.go"}}
	moreFiles := blocked
	moreFiles.MergeBlockFiles = []string{"a.go", "b.go"}
	tests := []struct {
		name      string
		old, next map[string][]tickets.Epic
		want      []string
	}{
		{"unchanged", scan("Plan", none), scan("Plan", none), nil},
		{"step moved", scan("Plan", none), scan("Branch", none), []string{"p:design"}},
		{"step first set", scan("", none), scan("Plan", none), []string{"p:design"}},
		{"step cleared", scan("Plan", none), scan("", none), []string{"p:design"}},
		{"merge paused", scan("", none), scan("", blocked), []string{"p:loop"}},
		{"merge block cleared", scan("", blocked), scan("", none), []string{"p:loop"}},
		{"block files changed", scan("", blocked), scan("", moreFiles), []string{"p:loop"}},
		{"block unchanged", scan("", blocked), scan("", blocked), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := changedEpicMeta(tc.old, tc.next)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
