package workspaces

import (
	"fmt"
	"slices"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// holdTarget is the key a hold is stored under and the epics whose queue
// membership blocks it.
type holdTarget struct {
	key    string
	design bool
	roots  []string
}

// holdTargetOf resolves a Design, its `-impl` epic or a loop epic. A refusal
// comes back as a result.
func (s *Store) holdTargetOf(addr string) (holdTarget, *wire.HoldResult) {
	refuse := func(reason, msg string) (holdTarget, *wire.HoldResult) {
		return holdTarget{}, &wire.HoldResult{Held: s.HeldKeys(), Refused: true, Reason: reason, Message: msg}
	}
	addr = strings.TrimSpace(addr)
	if s.dir == nil {
		return refuse(wire.ReasonHoldUnknown, string(ReasonUnknownKey))
	}
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
	if d, ok := strings.CutSuffix(addr, tickets.ImplEpicSuffix); ok && s.dir.DesignExists(d) {
		design = d
	}
	if s.dir.DesignExists(design) {
		return holdTarget{key: design, design: true, roots: []string{design, design + tickets.ImplEpicSuffix}}, nil
	}
	return holdTarget{key: addr, roots: []string{addr}}, nil
}

// HoldEpic holds a Design (by either address) or a loop epic. Holding a Design
// exits its live sessions, which needs yes; the result lists them.
func (s *Store) HoldEpic(addr string, yes bool) (wire.HoldResult, error) {
	t, refused := s.holdTargetOf(addr)
	if refused != nil {
		return *refused, nil
	}
	if slices.ContainsFunc(t.roots, s.dir.RootQueued) {
		return wire.HoldResult{Held: s.HeldKeys(), Refused: true, Reason: wire.ReasonHoldQueued, Message: wire.HoldQueuedText}, nil
	}
	var live []string
	if t.design {
		for _, si := range s.dir.Sessions() {
			if si.Design && si.Status.State != wire.SessionExited && tickets.EpicAddress(si.Ticket) == t.key {
				live = append(live, si.ID)
			}
		}
	}
	if len(live) > 0 && !yes {
		return wire.HoldResult{
			Held: s.HeldKeys(), Sessions: live, Refused: true, Reason: wire.ReasonHoldLiveSessions,
			Message: fmt.Sprintf("holding exits the live Design sessions: %s; confirm to continue", strings.Join(live, ", ")),
		}, nil
	}
	var ended []string
	for _, id := range live {
		if err := s.dir.StopSession(id); err != nil {
			return wire.HoldResult{}, err
		}
		ended = append(ended, id)
	}
	if err := s.Hold(t.key); err != nil {
		return wire.HoldResult{}, err
	}
	return wire.HoldResult{Held: s.HeldKeys(), Sessions: ended}, nil
}

// UnholdEpic clears the hold on a Design or epic. It asks nothing and starts nothing.
func (s *Store) UnholdEpic(addr string) (wire.HoldResult, error) {
	t, refused := s.holdTargetOf(addr)
	if refused != nil {
		return *refused, nil
	}
	if err := s.Unhold(t.key); err != nil {
		return wire.HoldResult{}, err
	}
	return wire.HoldResult{Held: s.HeldKeys()}, nil
}
