package server

import (
	"testing"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

type fakeLandingHost struct {
	Host
	target string
	fresh  tickets.Ticket
}

func (f fakeLandingHost) mergeTarget(tickets.Address) (string, error) { return f.target, nil }
func (f fakeLandingHost) FreshTicket(tickets.Address) (tickets.Epic, tickets.Ticket, bool) {
	return tickets.Epic{}, f.fresh, true
}

func TestLandingMergeModeResolvesTargetThroughHost(t *testing.T) {
	l := &Landing{host: fakeLandingHost{target: "develop"}}
	m := l.IterationModeFor(tickets.Address{Project: "p", Epic: "e", ID: "01"}, tickets.Ticket{Type: "merge"})
	got, err := m.target()
	if err != nil || got != "develop" || !m.Feature {
		t.Fatalf("target = %q, %v, feature = %v; want develop, nil, true", got, err, m.Feature)
	}
}

type fakeMergerHost struct {
	Host
	base, target string
}

func (f fakeMergerHost) rootRefs(string, []tickets.Epic, string, tickets.Ticket, string) (string, string, error) {
	return f.base, f.target, nil
}
func (f fakeMergerHost) hostingDeps(tickets.Address) ralphloop.Deps { return ralphloop.Deps{} }

func TestMergerEpicRefsAsksHostForRefs(t *testing.T) {
	m := &Merger{host: fakeMergerHost{base: "a", target: "b"}}
	_, base, target, err := m.EpicRefs("p", nil, "e", "/repo")
	if err != nil || base != "a" || target != "b" {
		t.Fatalf("refs = %q, %q, %v; want a, b, nil", base, target, err)
	}
}
