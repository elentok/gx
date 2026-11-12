package epics

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/elentok/gx/server/wire"
)

const seenFileName = "epic-seen.json"

// Seen states: the only two a card can be unread in.
const (
	seenNeedsYou = "needs-you"
	seenMerged   = "merged"
)

// SeenStore keeps, per Implementation card, the last Needs-you or Merged state
// a person saw. A card is unread while it sits in one of those states and the
// stored state differs. Leaving both states clears the entry, so a card that
// comes back to Needs you is unread again. Safe for concurrent use.
type SeenStore struct {
	mu   sync.Mutex
	path string
	st   seenState
}

type seenState struct {
	// Seeded is set once the first scan has marked the cards that already sat in
	// a state, so installing this version does not bold every card.
	Seeded bool              `json:"seeded"`
	Seen   map[string]string `json:"seen"`
}

// OpenSeen loads the store from stateDir; a missing or unreadable file starts empty.
func OpenSeen(stateDir string) *SeenStore {
	s := &SeenStore{path: filepath.Join(stateDir, seenFileName), st: seenState{Seen: map[string]string{}}}
	if b, err := os.ReadFile(s.path); err == nil {
		var st seenState
		if json.Unmarshal(b, &st) == nil && st.Seen != nil {
			s.st = st
		}
	}
	return s
}

// seenStateOf is the card's Needs-you or Merged state, "" in any other.
func seenStateOf(r wire.EpicListRow) string {
	if r.CardKind != wire.CardKindImplementation {
		return ""
	}
	switch {
	case r.Section == wire.SectionNeedsYou:
		return seenNeedsYou
	case r.State == wire.EpicListFinished && r.Section == wire.SectionFinished:
		return seenMerged
	}
	return ""
}

// Apply sets Unread on the cards and drops the entries of cards that left both
// states. The first call seeds every card already in a state as seen.
func (s *SeenStore) Apply(rows []wire.EpicListRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := maps.Clone(s.st.Seen)
	seeding := !s.st.Seeded
	for i := range rows {
		cur := seenStateOf(rows[i])
		key := rows[i].Address
		switch {
		case cur == "":
			delete(next, key)
		case seeding:
			next[key] = cur
		default:
			rows[i].Unread = next[key] != cur
		}
	}
	if !seeding && maps.Equal(next, s.st.Seen) {
		return nil
	}
	return s.save(seenState{Seeded: true, Seen: next})
}

// Mark records the card's current state as seen. cur is "" when it is in
// neither state, which clears the entry.
func (s *SeenStore) Mark(address, cur string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := maps.Clone(s.st.Seen)
	if cur == "" {
		delete(next, address)
	} else {
		next[address] = cur
	}
	if maps.Equal(next, s.st.Seen) {
		return nil
	}
	return s.save(seenState{Seeded: s.st.Seeded, Seen: next})
}

func (s *SeenStore) save(st seenState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.st = st
	return nil
}

// MarkSeen records the current state of the card at address as seen. An
// unknown address is not an error: there is nothing to mark.
func (s *Epics) MarkSeen(address string) error {
	if s.Seen == nil {
		return nil
	}
	list, err := s.cardList()
	if err != nil {
		return err
	}
	for _, r := range list.Epics {
		if r.Address == address || slices.Contains(r.Addresses, address) {
			return s.Seen.Mark(r.Address, seenStateOf(r))
		}
	}
	return nil
}
