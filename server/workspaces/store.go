// Package workspaces keeps the ordered workspaces, which Design, epic or chat
// belongs to which, and the hold marks. It is a leaf: callers supply a Resolver
// for the one question it cannot answer itself.
package workspaces

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// DefaultName is the Default workspace: always present, never deleted or renamed.
const DefaultName = "Default workspace"

const fileName = "workspaces.json"

// Reason says why a write was refused. Refusals are results, not panics.
type Reason string

const (
	ReasonBlankName    Reason = "workspace name is blank"
	ReasonNameTaken    Reason = "a workspace with that name already exists"
	ReasonUnknown      Reason = "no such workspace"
	ReasonDefaultFixed Reason = "the Default workspace cannot be renamed or deleted"
	ReasonBlankKey     Reason = "key is blank"
	ReasonBadOrder     Reason = "the order must list every workspace exactly once"
	ReasonUnknownKey   Reason = "no such Design, epic or chat"
	ReasonFollowsEpic  Reason = "a Queue or Design agent follows its epic; move the epic instead"
	ReasonNotJoinable  Reason = "only a Design, an epic or a chat can join a workspace"
	ReasonBadSort      Reason = "the sort must be one of needs-you, activity or name"
)

func (r Reason) Error() string { return string(r) }

type state struct {
	Order   []string          `json:"order"`
	Members map[string]string `json:"members"`
	// Sorts maps a workspace to its non-default Sort; absent means the default.
	Sorts map[string]string `json:"sorts,omitempty"`
	Held  []string          `json:"held"`
}

func (s state) clone() state {
	return state{
		Order:   slices.Clone(s.Order),
		Members: maps.Clone(s.Members),
		Sorts:   maps.Clone(s.Sorts),
		Held:    slices.Clone(s.Held),
	}
}

func validSort(v string) bool {
	return v == wire.SortNeedsYou || v == wire.SortActivity || v == wire.SortName || v == wire.SortProject || v == wire.SortStatus
}

// sortOf reads a workspace's Sort; a missing entry or an unknown value is the default.
func (s *Store) sortOf(canonical string) string {
	if v := s.st.Sorts[canonical]; validSort(v) {
		return v
	}
	return wire.SortDefault
}

// Sort returns a workspace's Sort, the default when none is set.
func (s *Store) Sort(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.index(name); i >= 0 {
		return s.sortOf(s.st.Order[i])
	}
	return wire.SortDefault
}

// SetSort sets a workspace's Sort; the default value clears the entry.
func (s *Store) SetSort(name, value string) error {
	return s.write(func() error {
		i := s.index(name)
		switch {
		case i < 0:
			return ReasonUnknown
		case !validSort(value):
			return ReasonBadSort
		}
		canonical := s.st.Order[i]
		delete(s.st.Sorts, canonical)
		if value != wire.SortDefault {
			if s.st.Sorts == nil {
				s.st.Sorts = map[string]string{}
			}
			s.st.Sorts[canonical] = value
		}
		return nil
	})
}

// Store is the workspace store. All methods are safe for concurrent use.
type Store struct {
	mu     sync.Mutex
	path   string
	events *broker.Broker
	dir    func() Directory
	st     state
}

// Reorder sets the workspace order; order must name every workspace exactly once.
func (s *Store) Reorder(order []string) error {
	return s.write(func() error {
		if len(order) != len(s.st.Order) {
			return ReasonBadOrder
		}
		next := make([]string, 0, len(order))
		for _, n := range order {
			i := s.index(n)
			if i < 0 {
				return ReasonUnknown
			}
			if slices.Contains(next, s.st.Order[i]) {
				return ReasonBadOrder
			}
			next = append(next, s.st.Order[i])
		}
		s.st.Order = next
		return nil
	})
}

// Join puts a Design, epic or chat in a workspace, refusing any other key.
func (s *Store) Join(key, workspace string) error {
	// The check shares the write's lock hold, so a delete cannot slip between them.
	// The directory (index, queue) never calls back into the store, as canon already relies on.
	return s.write(func() error {
		if err := s.joinable(key); err != nil {
			return err
		}
		return s.setMemberLocked(key, workspace)
	})
}

// Open loads the persisted store from stateDir. dir is called on each lookup,
// so the server can hand over itself after it is built.
func Open(stateDir string, events *broker.Broker, dir func() Directory) (*Store, error) {
	s := &Store{path: filepath.Join(stateDir, fileName), events: events, dir: dir}
	data, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, &s.st); err != nil {
			return nil, err
		}
	}
	if s.index(DefaultName) < 0 {
		s.st.Order = append([]string{DefaultName}, s.st.Order...)
	}
	if s.st.Members == nil {
		s.st.Members = map[string]string{}
	}
	return s, nil
}

// Listed returns the workspaces in order with their member counts.
func (s *Store) Listed() []wire.Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := map[string]int{}
	for _, ws := range s.st.Members {
		counts[ws]++
	}
	out := make([]wire.Workspace, 0, len(s.st.Order))
	for _, n := range s.st.Order {
		out = append(out, wire.Workspace{Name: n, Default: n == DefaultName, Members: counts[n], Sort: s.sortOf(n)})
	}
	return out
}

// Workspace returns the workspace a key belongs to; an unknown key is in the Default workspace.
func (s *Store) Workspace(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name, ok := s.st.Members[s.canon(key)]; ok {
		return name
	}
	return DefaultName
}

// IsHeld reports whether key carries a hold mark.
func (s *Store) IsHeld(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Contains(s.st.Held, s.canon(key))
}

// HeldKeys lists the held keys.
func (s *Store) HeldKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.st.Held)
}

// Create adds a workspace last.
func (s *Store) Create(name string) error {
	name = strings.TrimSpace(name)
	return s.write(func() error {
		if name == "" {
			return ReasonBlankName
		}
		if s.index(name) >= 0 {
			return ReasonNameTaken
		}
		s.st.Order = append(s.st.Order, name)
		return nil
	})
}

// Rename renames a workspace; its members follow.
func (s *Store) Rename(from, to string) error {
	to = strings.TrimSpace(to)
	return s.write(func() error {
		i := s.index(from)
		switch {
		case i < 0:
			return ReasonUnknown
		case s.st.Order[i] == DefaultName:
			return ReasonDefaultFixed
		case to == "":
			return ReasonBlankName
		}
		if j := s.index(to); j >= 0 && j != i {
			return ReasonNameTaken
		}
		old := s.st.Order[i]
		s.st.Order[i] = to
		for k, ws := range s.st.Members {
			if ws == old {
				s.st.Members[k] = to
			}
		}
		if v, ok := s.st.Sorts[old]; ok {
			delete(s.st.Sorts, old)
			s.st.Sorts[to] = v
		}
		return nil
	})
}

// Delete removes a workspace and moves its members to the Default workspace.
func (s *Store) Delete(name string) error {
	return s.write(func() error {
		i := s.index(name)
		switch {
		case i < 0:
			return ReasonUnknown
		case s.st.Order[i] == DefaultName:
			return ReasonDefaultFixed
		}
		old := s.st.Order[i]
		s.st.Order = slices.Delete(s.st.Order, i, i+1)
		for k, ws := range s.st.Members {
			if ws == old {
				delete(s.st.Members, k)
			}
		}
		delete(s.st.Sorts, old)
		return nil
	})
}

// Move puts a workspace at position to (clamped to the list); Default can move too.
func (s *Store) Move(name string, to int) error {
	return s.write(func() error {
		i := s.index(name)
		if i < 0 {
			return ReasonUnknown
		}
		n := s.st.Order[i]
		s.st.Order = slices.Delete(s.st.Order, i, i+1)
		to = max(0, min(to, len(s.st.Order)))
		s.st.Order = slices.Insert(s.st.Order, to, n)
		return nil
	})
}

// SetMember puts key in a workspace. The Default workspace stores nothing.
func (s *Store) SetMember(key, workspace string) error {
	return s.write(func() error { return s.setMemberLocked(key, workspace) })
}

// setMemberLocked is SetMember's body; the caller holds s.mu through write.
func (s *Store) setMemberLocked(key, workspace string) error {
	key = s.canon(key)
	if strings.TrimSpace(key) == "" {
		return ReasonBlankKey
	}
	i := s.index(workspace)
	if i < 0 {
		return ReasonUnknown
	}
	if s.st.Order[i] == DefaultName {
		delete(s.st.Members, key)
	} else {
		s.st.Members[key] = s.st.Order[i]
	}
	return nil
}

// Remove returns key to the Default workspace.
func (s *Store) Remove(key string) error {
	return s.write(func() error {
		delete(s.st.Members, s.canon(key))
		return nil
	})
}

// Hold marks key held; the store only keeps the mark.
func (s *Store) Hold(key string) error {
	return s.write(func() error {
		key = s.canon(key)
		if strings.TrimSpace(key) == "" {
			return ReasonBlankKey
		}
		if !slices.Contains(s.st.Held, key) {
			s.st.Held = append(s.st.Held, key)
		}
		return nil
	})
}

// Unhold clears the hold mark on key.
func (s *Store) Unhold(key string) error {
	return s.write(func() error {
		key = s.canon(key)
		s.st.Held = slices.DeleteFunc(s.st.Held, func(k string) bool { return k == key })
		return nil
	})
}

// Prune drops members and holds whose key keep rejects.
func (s *Store) Prune(keep func(key string) bool) error {
	return s.write(func() error {
		for k := range s.st.Members {
			if !keep(k) {
				delete(s.st.Members, k)
			}
		}
		s.st.Held = slices.DeleteFunc(s.st.Held, func(k string) bool { return !keep(k) })
		return nil
	})
}

// index finds a workspace name without regard to case.
func (s *Store) index(name string) int {
	name = strings.TrimSpace(name)
	return slices.IndexFunc(s.st.Order, func(n string) bool { return strings.EqualFold(n, name) })
}

// canon maps a Design's `-impl` key to the Design's key, so both read one workspace and hold.
func (s *Store) canon(key string) string {
	if design, ok := strings.CutSuffix(key, tickets.ImplEpicSuffix); ok && s.dir().DesignExists(design) {
		return design
	}
	return key
}

// write applies fn, saves, and publishes. A refused or failed write changes
// nothing and publishes nothing.
func (s *Store) write(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.st.clone()
	if err := fn(); err != nil {
		s.st = prev
		return err
	}
	// Rescans and session exits prune on every pass; a no-op must not wake the web.
	if reflect.DeepEqual(prev, s.st) {
		return nil
	}
	if err := s.save(); err != nil {
		s.st = prev
		return err
	}
	s.events.Publish(wire.EventWorkspacesChanged, "")
	return nil
}

// save writes through a rename so a crash never leaves a torn file.
func (s *Store) save() error {
	data, err := json.Marshal(s.st)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
