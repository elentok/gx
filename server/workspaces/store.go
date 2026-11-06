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
)

func (r Reason) Error() string { return string(r) }

// Resolver answers whether a Design exists, given its `project:epic` key.
type Resolver interface {
	DesignExists(key string) bool
}

type state struct {
	Order   []string          `json:"order"`
	Members map[string]string `json:"members"`
	Held    []string          `json:"held"`
}

func (s state) clone() state {
	return state{
		Order:   slices.Clone(s.Order),
		Members: maps.Clone(s.Members),
		Held:    slices.Clone(s.Held),
	}
}

// Store is the workspace store. All methods are safe for concurrent use.
type Store struct {
	mu       sync.Mutex
	path     string
	events   *broker.Broker
	resolver Resolver
	st       state
}

// Open loads the persisted store from stateDir. resolver may be nil, in which
// case no `-impl` key is folded into a Design.
func Open(stateDir string, events *broker.Broker, resolver Resolver) (*Store, error) {
	s := &Store{path: filepath.Join(stateDir, fileName), events: events, resolver: resolver}
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

// List returns the workspace names in order.
func (s *Store) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.st.Order)
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
		out = append(out, wire.Workspace{Name: n, Default: n == DefaultName, Members: counts[n]})
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
	return s.write(func() error {
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
	})
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
	if s.resolver == nil {
		return key
	}
	if design, ok := strings.CutSuffix(key, tickets.ImplEpicSuffix); ok && s.resolver.DesignExists(design) {
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
