package schema

import (
	"reflect"
	"testing"
)

func TestSumFriction(t *testing.T) {
	a := FrictionSummary{
		PausesByReason:  map[string]int{"zero-commit": 1, "unknown": 1},
		RunEvents:       map[string]int{"conflict-hit": 1},
		PauseEndings:    map[string]int{"web": 1},
		TotalPausedSecs: 10,
		PersonPauseSecs: []int{10},
	}
	b := FrictionSummary{
		PausesByReason:  map[string]int{"zero-commit": 2},
		PauseEndings:    map[string]int{"web": 1, "gx": 1},
		TotalPausedSecs: 5,
		PersonPauseSecs: []int{3},
	}
	want := FrictionSummary{
		PausesByReason:  map[string]int{"zero-commit": 3, "unknown": 1},
		RunEvents:       map[string]int{"conflict-hit": 1},
		PauseEndings:    map[string]int{"web": 2, "gx": 1},
		TotalPausedSecs: 15,
		PersonPauseSecs: []int{10, 3},
	}
	if got := SumFriction(a, b); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if got := SumFriction(); !reflect.DeepEqual(got, FrictionSummary{}) {
		t.Errorf("empty sum = %+v", got)
	}
}
