package workspaces

import (
	"cmp"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// Directory is what the membership rules and placement need from the server:
// the epic behind a `project:epic` key, whether it is a Design, and the sessions.
type Directory interface {
	// DesignExists reports whether a Design exists, given its `project:epic` key.
	DesignExists(key string) bool
	Epic(key string) (tickets.Epic, bool)
	Sessions() []wire.SessionInfo
	// RootQueued reports whether any ticket of the `project:epic` root is queued.
	RootQueued(key string) bool
	// StopSession ends a session.
	StopSession(id string) error
}

// joinable refuses every key but a Design, an epic or a plain chat.
func (s *Store) joinable(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return ReasonBlankKey
	}
	dir := s.dir()
	if _, name, ok := tickets.ParseEpicAddress(key); ok {
		if name == tickets.FollowUpsEpic || name == tickets.ArchivedFollowUpsEpic || tickets.IsOneOffsEpicName(name) {
			return ReasonNotJoinable
		}
		e, ok := dir.Epic(key)
		switch {
		case !ok:
			return ReasonUnknownKey
		case tickets.IsOneOffEpic(e):
			return ReasonNotJoinable
		}
		return nil
	}
	for _, si := range dir.Sessions() {
		if si.ID != key {
			continue
		}
		switch {
		case inOneOffs(si) || si.FixOf != "":
			return ReasonNotJoinable
		case si.Queue || si.Design:
			return ReasonFollowsEpic
		}
		return nil
	}
	return ReasonUnknownKey
}

func inOneOffs(si wire.SessionInfo) bool {
	a, err := tickets.ParseQueuedAddress(si.Ticket)
	return err == nil && tickets.IsOneOffsEpicName(a.Epic)
}

// Place says where each session sits. Rules stay here; the web only renders.
func (s *Store) Place(sessions []wire.SessionInfo) []wire.SessionPlacement {
	out := make([]wire.SessionPlacement, 0, len(sessions))
	for _, si := range sessions {
		out = append(out, s.place(si))
	}
	return out
}

func (s *Store) place(si wire.SessionInfo) wire.SessionPlacement {
	if !si.Design && !si.Queue && si.FixOf == "" {
		return wire.SessionPlacement{Session: si.ID, Kind: wire.PlacementChat, Workspace: s.Workspace(si.ID)}
	}
	other := wire.SessionPlacement{Session: si.ID, Kind: wire.PlacementOther}
	addr := cmp.Or(si.FixOf, si.Ticket)
	// An archived epic's agents match no sidebar row, so they show nowhere. Test
	// this before parsing: an archive key must not fall to Other.
	if tickets.IsArchiveKey(addr) {
		return wire.SessionPlacement{Session: si.ID, Kind: wire.PlacementEpic, Epic: tickets.ArchiveEpicKey(addr)}
	}
	// A project-level Fix agent has a bare project name here, which does not parse.
	a, err := tickets.ParseQueuedAddress(addr)
	if err != nil || tickets.IsOneOffsEpicName(a.Epic) {
		return other
	}
	epic := a.Project + ":" + a.Epic
	dir := s.dir()
	if e, ok := dir.Epic(epic); ok && tickets.IsOneOffEpic(e) {
		return other
	}
	if design, ok := strings.CutSuffix(epic, tickets.ImplEpicSuffix); ok && dir.DesignExists(design) {
		epic = design
	}
	return wire.SessionPlacement{Session: si.ID, Kind: wire.PlacementEpic, Epic: epic}
}
