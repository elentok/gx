package notify

import "testing"

func TestTarget_PathMapsEachTargetToItsRoute(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		target Target
		want   string
	}{
		{"ticket", Target{Ticket: "proj:epic/01"}, "/t/proj:epic/01"},
		{"epic", Target{Epic: "proj:epic"}, "/e/proj:epic"},
		{"design", Target{Design: "proj:map"}, "/d/proj:map"},
		{"session", Target{Session: "abc"}, "/agents/abc"},
		{"none", Target{}, "/"},
		{"ticket wins over epic", Target{Ticket: "proj:epic/01", Epic: "proj:epic"}, "/t/proj:epic/01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.target.Path(); got != tc.want {
				t.Errorf("Path() = %q, want %q", got, tc.want)
			}
		})
	}
}
