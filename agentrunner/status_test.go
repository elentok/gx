package agentrunner

import (
	"errors"
	"fmt"
	"testing"
)

func TestStatus_Waiting(t *testing.T) {
	for state, want := range map[State]bool{StateIdle: true, StateDone: true, StateWorking: false, StateBlocked: false, "": false} {
		if got := (Status{State: state}).Waiting(); got != want {
			t.Errorf("Waiting(%q) = %v, want %v", state, got, want)
		}
	}
}

func TestLabelTakenError_IsErrLabelTakenAndCarriesCwd(t *testing.T) {
	err := fmt.Errorf("start: %w", &LabelTakenError{HolderCwd: "/w", Err: errors.New("host says no")})
	if !errors.Is(err, ErrLabelTaken) {
		t.Errorf("errors.Is(err, ErrLabelTaken) = false")
	}
	var taken *LabelTakenError
	if !errors.As(err, &taken) || taken.HolderCwd != "/w" {
		t.Errorf("errors.As did not yield a LabelTakenError with cwd /w: %+v", taken)
	}
}
