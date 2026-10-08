package tickets

import (
	"errors"
	"slices"
	"testing"
)

func TestDeriveLeafBase(t *testing.T) {
	sib := func(n int, id, status string, commitless bool) Ticket {
		return Ticket{Number: n, Identifier: id, Status: status, Commitless: commitless}
	}
	leaf := func(blockedBy ...string) Ticket {
		return Ticket{Number: 3, Identifier: "03", Status: "open", BlockedBy: blockedBy}
	}
	for name, tc := range map[string]struct {
		tickets   []Ticket
		leaf      Ticket
		want      string
		ambiguous []string
	}{
		"unlanded sibling is the base":   {tickets: []Ticket{sib(1, "01", "open", false)}, leaf: leaf("01"), want: "01"},
		"landed sibling gives the tip":   {tickets: []Ticket{sib(1, "01", "done", false)}, leaf: leaf("01")},
		"commitless sibling is ignored":  {tickets: []Ticket{sib(1, "01", "open", true)}, leaf: leaf("01")},
		"commitless ticket reads at tip": {tickets: []Ticket{sib(1, "01", "open", false)}, leaf: func() Ticket { l := leaf("01"); l.Commitless = true; return l }()},
		"explicit base opts out":         {tickets: []Ticket{sib(1, "01", "open", false)}, leaf: func() Ticket { l := leaf("01"); l.Base = "x"; return l }()},
		"two unlanded siblings are ambiguous": {
			tickets: []Ticket{sib(1, "01", "open", false), sib(2, "02", "claimed", false)}, leaf: leaf("01", "02"), ambiguous: []string{"01", "02"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			epics := []Epic{{Name: "a", Tickets: append(slices.Clone(tc.tickets), tc.leaf)}}
			got, err := DeriveLeafBase("p", epics, "a", tc.leaf)
			var amb *AmbiguousBaseError
			if errors.As(err, &amb) != (tc.ambiguous != nil) || (amb != nil && !slices.Equal(amb.Blockers, tc.ambiguous)) || (amb == nil && err != nil) || got != tc.want {
				t.Errorf("got (%q, %v), want (%q, ambiguous=%v)", got, err, tc.want, tc.ambiguous)
			}
		})
	}
}

func baseEpics(blockerStatus string) []Epic {
	return []Epic{
		{Name: "a", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: blockerStatus}}},
		{Name: "b", BlockedBy: []string{"a/01"}, Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}},
	}
}

func TestDeriveRootBase_OnLandedIsAlwaysTrunk(t *testing.T) {
	e := baseEpics("open")
	got, err := DeriveRootBase("p", e, "b", e[1].Tickets[0], true)
	if err != nil || got != "" {
		t.Errorf("got (%q, %v), want trunk", got, err)
	}
	e[1].Base = "release/1.0"
	if got, _ := DeriveRootBase("p", e, "b", e[1].Tickets[0], true); got != "release/1.0" {
		t.Errorf("explicit base lost: %q", got)
	}
}

func TestDeriveRootBase(t *testing.T) {
	for name, tc := range map[string]struct {
		epics   []Epic
		want    string
		wantErr bool
	}{
		"unlanded blocker gives its branch": {epics: baseEpics("open"), want: "a"},
		"landed blocker gives trunk":        {epics: baseEpics("done"), want: ""},
		"no blockers gives trunk": {epics: func() []Epic {
			e := baseEpics("open")
			e[1].BlockedBy = nil
			return e
		}(), want: ""},
		"commitless blocker is ignored": {epics: func() []Epic {
			e := baseEpics("open")
			e[0].Tickets[0].Commitless = true
			return e
		}(), want: ""},
		"two unlanded blockers are ambiguous": {epics: func() []Epic {
			e := append(baseEpics("open"), Epic{Name: "c", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}})
			e[1].BlockedBy = []string{"a/01", "c/01"}
			return e
		}(), wantErr: true},
		"base node ref unlanded": {epics: func() []Epic {
			e := baseEpics("open")
			e[1].BlockedBy, e[1].Base = nil, "a/01"
			return e
		}(), want: "a"},
		"base node ref landed": {epics: func() []Epic {
			e := baseEpics("done")
			e[1].BlockedBy, e[1].Base = nil, "a/01"
			return e
		}(), want: ""},
		"raw branch base": {epics: func() []Epic {
			e := baseEpics("open")
			e[1].Base = "release/1.0"
			return e
		}(), want: "release/1.0"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := DeriveRootBase("p", tc.epics, "b", tc.epics[1].Tickets[0], false)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("got (%q, %v), want (%q, err=%v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
