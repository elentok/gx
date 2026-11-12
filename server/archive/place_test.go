package archive

import (
	"errors"
	"testing"

	"github.com/elentok/gx/server/workspaces"
)

func TestPlaceInWorkspace_OnlyAGoneWorkspaceFallsBackToDefault(t *testing.T) {
	ioErr := errors.New("disk full")
	cases := []struct {
		name       string
		setMember  error
		wantErr    error
		wantRemove bool
	}{
		{"placed", nil, nil, false},
		{"workspace gone", workspaces.ReasonUnknown, nil, true},
		{"other error", ioErr, ioErr, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			removed := false
			a := New(Deps{
				SetMember: func(string, string) error { return c.setMember },
				Remove:    func(string) error { removed = true; return nil },
			})
			err := a.placeInWorkspace("p:e", "Bugs")
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil) != (err == nil) {
				t.Errorf("err = %v, want %v", err, c.wantErr)
			}
			if removed != c.wantRemove {
				t.Errorf("removed = %v, want %v", removed, c.wantRemove)
			}
		})
	}
}
