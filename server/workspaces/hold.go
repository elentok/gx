package workspaces

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// SessionKinds picks which of an epic's live Sessions a stop reaches.
type SessionKinds uint8

const (
	// OwnSessions are the epic's queue agents.
	OwnSessions SessionKinds = 1 << iota
	// FixSessions are the Fix agents of the epic's tickets.
	FixSessions
	// DesignSessions are the Design's Sessions.
	DesignSessions
)

// LiveEpicSessions lists the ids of an epic's live Sessions of the given kinds.
func LiveEpicSessions(dir Directory, epic string, kinds SessionKinds) []string {
	var live []string
	for _, si := range dir.Sessions() {
		if si.Status.State == wire.SessionExited || !kinds.has(si) {
			continue
		}
		if tickets.EpicAddress(cmp.Or(si.FixOf, si.Ticket)) == epic {
			live = append(live, si.ID)
		}
	}
	return live
}

func (k SessionKinds) has(si wire.SessionInfo) bool {
	switch {
	case si.Design:
		return k&DesignSessions != 0
	case si.FixOf != "":
		return k&FixSessions != 0
	case si.Queue:
		return k&OwnSessions != 0
	}
	return false
}

// StopEpicSessions stops an epic's live Sessions of the given kinds and returns their ids.
// It is the one stop path: Hold uses it for Design Sessions, Archive for all three kinds.
func StopEpicSessions(dir Directory, epic string, kinds SessionKinds) ([]string, error) {
	live := LiveEpicSessions(dir, epic, kinds)
	for _, id := range live {
		if err := dir.StopSession(id); err != nil {
			return nil, err
		}
	}
	return live, nil
}

// holdTarget is the key a hold is stored under and the epics whose queue
// membership blocks it.
type holdTarget struct {
	key    string
	design bool
	roots  []string
}

// holdTargetOf resolves a Design, its `-impl` epic or a loop epic. A refusal
// comes back as a result.
func (s *Store) holdTargetOf(addr string) (holdTarget, *wire.TargetResult) {
	refuse := func(reason, msg string) (holdTarget, *wire.TargetResult) {
		r := wire.RefusedTarget(addr, reason, msg)
		return holdTarget{}, &r
	}
	addr = strings.TrimSpace(addr)
	dir := s.dir()
	if _, _, ok := tickets.ParseEpicAddress(addr); !ok {
		return refuse(wire.ReasonHoldUnknown, string(ReasonUnknownKey))
	}
	if err := s.joinable(addr); err != nil {
		if err == ReasonNotJoinable {
			return refuse(wire.ReasonHoldNotHoldable, "only a Design or an epic can be held")
		}
		return refuse(wire.ReasonHoldUnknown, err.Error())
	}
	design := addr
	if d, ok := strings.CutSuffix(addr, tickets.ImplEpicSuffix); ok && dir.DesignExists(d) {
		design = d
	}
	if dir.DesignExists(design) {
		return holdTarget{key: design, design: true, roots: []string{design, design + tickets.ImplEpicSuffix}}, nil
	}
	return holdTarget{key: addr, roots: []string{addr}}, nil
}

// HoldEpics holds each Design (by either address) or loop epic on its own, in
// order. A Design's live sessions exit, which needs yes for the whole request;
// without it that target is refused and the others still run. A dry run writes
// nothing and answers fits with the sessions that would exit.
func (s *Store) HoldEpics(targets []string, yes, dryRun bool) (wire.TargetsResult, error) {
	res := wire.TargetsResult{Results: make([]wire.TargetResult, 0, len(targets))}
	for _, addr := range targets {
		r, err := s.holdOne(addr, yes, dryRun)
		if err != nil {
			return wire.TargetsResult{}, err
		}
		res.Results = append(res.Results, r)
	}
	res.Held = s.HeldKeys()
	return res, nil
}

func (s *Store) holdOne(addr string, yes, dryRun bool) (wire.TargetResult, error) {
	t, refused := s.holdTargetOf(addr)
	if refused != nil {
		return *refused, nil
	}
	dir := s.dir()
	if slices.ContainsFunc(t.roots, dir.RootQueued) {
		return wire.RefusedTarget(addr, wire.ReasonHoldQueued, wire.HoldQueuedText), nil
	}
	var live []string
	if t.design {
		live = LiveEpicSessions(dir, t.key, DesignSessions)
	}
	if dryRun {
		return wire.TargetResult{Target: addr, Status: wire.TargetFits, Sessions: live}, nil
	}
	if len(live) > 0 && !yes {
		r := wire.RefusedTarget(addr, wire.ReasonNeedsYes,
			fmt.Sprintf("holding exits the live Design sessions: %s; confirm to continue", strings.Join(live, ", ")))
		r.Sessions = live
		return r, nil
	}
	var ended []string
	if t.design {
		var err error
		if ended, err = StopEpicSessions(dir, t.key, DesignSessions); err != nil {
			return wire.TargetResult{}, err
		}
	}
	if err := s.Hold(t.key); err != nil {
		return wire.TargetResult{}, err
	}
	return wire.TargetResult{Target: addr, Status: wire.TargetDone, Sessions: ended}, nil
}

// UnholdEpics clears the hold on each Design or epic. It asks nothing and starts nothing.
func (s *Store) UnholdEpics(targets []string) (wire.TargetsResult, error) {
	res := wire.TargetsResult{Results: make([]wire.TargetResult, 0, len(targets))}
	for _, addr := range targets {
		t, refused := s.holdTargetOf(addr)
		if refused != nil {
			res.Results = append(res.Results, *refused)
			continue
		}
		if !s.IsHeld(t.key) {
			res.Results = append(res.Results, wire.RefusedTarget(addr, wire.ReasonNotOnHold, "not on hold"))
			continue
		}
		if err := s.Unhold(t.key); err != nil {
			return wire.TargetsResult{}, err
		}
		res.Results = append(res.Results, wire.TargetResult{Target: addr, Status: wire.TargetDone})
	}
	res.Held = s.HeldKeys()
	return res, nil
}

// MoveTargets puts each key in the workspace on its own, in order.
func (s *Store) MoveTargets(keys []string, workspace string) (wire.TargetsResult, error) {
	res := wire.TargetsResult{Results: make([]wire.TargetResult, 0, len(keys))}
	for _, key := range keys {
		err := s.Join(key, workspace)
		var reason Reason
		switch {
		case err == nil:
			res.Results = append(res.Results, wire.TargetResult{Target: key, Status: wire.TargetDone})
		case errors.As(err, &reason):
			res.Results = append(res.Results, wire.RefusedTarget(key, refusals[reason], reason.Error()))
		default:
			return wire.TargetsResult{}, err
		}
	}
	return res, nil
}
