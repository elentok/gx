package server

import (
	"reflect"
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestChangedEpicDesignSteps(t *testing.T) {
	scan := func(step string) map[string][]tickets.Epic {
		return map[string][]tickets.Epic{
			"/store/p": {{Path: "/store/p/design", DesignStep: step}, {Path: "/store/p/loop"}},
		}
	}
	tests := []struct {
		name      string
		old, next map[string][]tickets.Epic
		want      []string
	}{
		{"unchanged", scan("Plan"), scan("Plan"), nil},
		{"step moved", scan("Plan"), scan("Branch"), []string{"p:design"}},
		{"step first set", scan(""), scan("Plan"), []string{"p:design"}},
		{"step cleared", scan("Plan"), scan(""), []string{"p:design"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := changedEpicDesignSteps(tc.old, tc.next)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
