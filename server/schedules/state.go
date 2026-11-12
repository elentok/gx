// Package schedules keeps the per-machine half of a Schedule: machine id,
// on/off switch, baseline and skip log. It lives in the server's state dir,
// never in the shared Schedule file, so two machines on one ticket store do
// not share it.
package schedules

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/fsutil"
)

const (
	stateDirName  = "schedules"
	stateFileName = "state.json"
	maxSkips      = 10
)

// Skip reasons.
const (
	ReasonLate                      = "late"
	ReasonProjectNotFound           = "project-not-found"
	ReasonPreviousRunStarted        = "previous-run-started"
	ReasonPreviousRunNeedsAttention = "previous-run-needs-attention"
)

// Key identifies one Schedule.
type Key struct {
	Project string
	Slug    string
}

func (k Key) String() string { return k.Project + "/" + k.Slug }

// Skip is one logged skipped set time.
type Skip struct {
	// SetTime is the set time the skip was for.
	SetTime  time.Time `json:"set_time"`
	Reason   string    `json:"reason"`
	LoggedAt time.Time `json:"logged_at"`
	// Blocking is the blocking ticket's address, for previous-run-needs-attention.
	Blocking string `json:"blocking,omitempty"`
}

type entry struct {
	On       bool      `json:"on,omitempty"`
	Baseline time.Time `json:"baseline,omitzero"`
	Skips    []Skip    `json:"skips,omitempty"`
}

type file struct {
	MachineID string           `json:"machine_id"`
	Entries   map[string]entry `json:"entries,omitempty"`
}

// State is the per-machine Schedule state. Safe for concurrent use.
type State struct {
	mu   sync.Mutex
	path string
	f    file
}

// Open loads the state from stateDir, creating a random machine id on first use.
func Open(stateDir string) (*State, error) { return OpenWithID(stateDir, "") }

// OpenWithID is Open, but uses id as the machine id when none is stored yet.
// An empty id means a random one.
func OpenWithID(stateDir, id string) (*State, error) {
	dir := filepath.Join(stateDir, stateDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &State{path: filepath.Join(dir, stateFileName)}
	data, err := os.ReadFile(s.path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &s.f); err != nil {
			return nil, err
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	if s.f.MachineID == "" {
		if id == "" {
			id, err = randomID()
			if err != nil {
				return nil, err
			}
		}
		s.f.MachineID = id
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func randomID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// MachineID returns the stable id for the Dedupe key.
func (s *State) MachineID() string { return s.f.MachineID }

func (s *State) save() error {
	data, err := json.Marshal(s.f)
	if err != nil {
		return err
	}
	return fsutil.WriteFilePrivateAtomic(s.path, data)
}

// mutate applies fn to the key's entry and saves; on a save error the
// in-memory state is rolled back.
func (s *State) mutate(k Key, fn func(e *entry)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, had := s.f.Entries[k.String()]
	prevSkips := append([]Skip(nil), prev.Skips...)
	e := prev
	fn(&e)
	if s.f.Entries == nil {
		s.f.Entries = map[string]entry{}
	}
	s.f.Entries[k.String()] = e
	if err := s.save(); err != nil {
		if had {
			prev.Skips = prevSkips
			s.f.Entries[k.String()] = prev
		} else {
			delete(s.f.Entries, k.String())
		}
		return err
	}
	return nil
}

func (s *State) get(k Key) entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.Entries[k.String()]
}

// Enabled reports the Schedule's switch on this machine; off when unset.
func (s *State) Enabled(k Key) bool { return s.get(k).On }

// Enable turns the switch on and sets the baseline to at.
func (s *State) Enable(k Key, at time.Time) error {
	return s.mutate(k, func(e *entry) { e.On = true; e.Baseline = at })
}

// Disable turns the switch off and leaves the baseline alone.
func (s *State) Disable(k Key) error {
	return s.mutate(k, func(e *entry) { e.On = false })
}

// Baseline returns the last-handled time; ok is false when none is set.
func (s *State) Baseline(k Key) (t time.Time, ok bool) {
	b := s.get(k).Baseline
	return b, !b.IsZero()
}

// SetBaseline sets the baseline unconditionally.
func (s *State) SetBaseline(k Key, t time.Time) error {
	return s.mutate(k, func(e *entry) { e.Baseline = t })
}

// Advance moves the baseline forward to t. It never moves it back.
func (s *State) Advance(k Key, t time.Time) error {
	if !t.After(s.get(k).Baseline) {
		return nil
	}
	return s.mutate(k, func(e *entry) {
		if t.After(e.Baseline) {
			e.Baseline = t
		}
	})
}

// AppendSkip logs a skip, keeping the newest 10 for the Schedule.
func (s *State) AppendSkip(k Key, sk Skip) error {
	return s.mutate(k, func(e *entry) {
		skips := append(append([]Skip(nil), e.Skips...), sk)
		if len(skips) > maxSkips {
			skips = skips[len(skips)-maxSkips:]
		}
		e.Skips = skips
	})
}

// Skips returns the Schedule's skip log, oldest first.
func (s *State) Skips(k Key) []Skip {
	return append([]Skip(nil), s.get(k).Skips...)
}

// Logged reports whether setTime was already logged with reason.
func (s *State) Logged(k Key, setTime time.Time, reason string) bool {
	for _, sk := range s.get(k).Skips {
		if sk.Reason == reason && sk.SetTime.Equal(setTime) {
			return true
		}
	}
	return false
}

// Delete removes every entry of one Schedule.
func (s *State) Delete(k Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, had := s.f.Entries[k.String()]
	if !had {
		return nil
	}
	delete(s.f.Entries, k.String())
	if err := s.save(); err != nil {
		s.f.Entries[k.String()] = prev
		return err
	}
	return nil
}

// Keys lists the Schedules that have any entry.
func (s *State) Keys() []Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Key, 0, len(s.f.Entries))
	for name := range s.f.Entries {
		out = append(out, splitKey(name))
	}
	return out
}

func splitKey(name string) Key {
	i := strings.LastIndexByte(name, '/')
	return Key{Project: name[:max(i, 0)], Slug: name[i+1:]}
}
