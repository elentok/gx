// Package chatdrafts keeps chat drafts: a brief and the launch settings of a
// chat that does not exist yet, one JSON file per draft in its own folder of
// the gx state dir.
package chatdrafts

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/server/wire/ops"
)

const (
	rootDir    = "chat-drafts"
	recordName = "draft.json"
)

// ErrNotFound: the draft id does not exist (or is not a well-formed id).
var ErrNotFound = errors.New("chat draft not found")

var idPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Store reads and writes drafts under <stateDir>/chat-drafts. Each draft lives
// in <id>/draft.json.
type Store struct {
	root string
	now  func() time.Time
	// locks maps a draft id to its *sync.Mutex.
	locks sync.Map
}

// New builds a Store rooted in stateDir.
func New(stateDir string) *Store {
	return &Store{root: filepath.Join(stateDir, rootDir), now: time.Now}
}

func (s *Store) dir(id string) string    { return filepath.Join(s.root, id) }
func (s *Store) record(id string) string { return filepath.Join(s.dir(id), recordName) }

func (s *Store) lock(id string) func() {
	m, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// NewID makes a draft id: 16 hex characters.
func NewID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

const (
	labelWords    = 5
	labelMaxRunes = 40
	labelFallback = "Untitled chat"
)

// Label is the one rule for a draft's display name: the name, else the first 5
// words of the brief cut at 40 characters, else "Untitled chat".
func Label(name, brief string) string {
	if name != "" {
		return name
	}
	words := strings.Fields(brief)
	if len(words) == 0 {
		return labelFallback
	}
	if len(words) > labelWords {
		words = words[:labelWords]
	}
	r := []rune(strings.Join(words, " "))
	return string(r[:min(len(r), labelMaxRunes)])
}

// Create stores d, whose ID came from NewID, at revision 1.
func (s *Store) Create(d *ops.ChatDraft) error {
	if !idPattern.MatchString(d.ID) {
		return fmt.Errorf("bad chat draft id %q", d.ID)
	}
	d.Revision = 1
	d.EditedAt = s.now().UTC()
	unlock := s.lock(d.ID)
	defer unlock()
	if err := s.write(d); err != nil {
		return err
	}
	d.Label = Label(d.Name, d.Brief)
	return nil
}

// Get reads a draft without changing its revision or edited-at.
func (s *Store) Get(id string) (*ops.ChatDraft, error) {
	if !idPattern.MatchString(id) {
		return nil, ErrNotFound
	}
	b, err := os.ReadFile(s.record(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d ops.ChatDraft
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("chat draft %s: %w", id, err)
	}
	d.Label = Label(d.Name, d.Brief)
	return &d, nil
}

// Exists reports whether a readable draft has this id.
func (s *Store) Exists(id string) bool {
	_, err := s.Get(id)
	return err == nil
}

// List returns every draft, most recently edited first. An unreadable draft is skipped.
func (s *Store) List() ([]ops.ChatDraft, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return []ops.ChatDraft{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := []ops.ChatDraft{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if d, err := s.Get(e.Name()); err == nil {
			items = append(items, *d)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].EditedAt.After(items[j].EditedAt) })
	return items, nil
}

// Save applies a save with no settings check. See SaveChecked.
func (s *Store) Save(req ops.ChatDraftSaveRequest) (ops.ChatDraftResult, error) {
	return s.SaveChecked(req, nil)
}

// SaveChecked writes the brief and merges each settings field the request
// carries. The record's Dir, WorktreePath and StartError are left to check,
// which sees the merged draft under the draft's lock and may fix up Dir;
// a non-nil result from it is the refusal and nothing is written. A stale
// revision or a missing draft is a refused result, not an error.
func (s *Store) SaveChecked(req ops.ChatDraftSaveRequest, check func(d *ops.ChatDraft, before ops.ChatDraft) *ops.ChatDraftResult) (ops.ChatDraftResult, error) {
	if !idPattern.MatchString(req.ID) {
		return ops.ChatDraftResult{Refused: true, Reason: ops.ReasonDraftGone}, nil
	}
	unlock := s.lock(req.ID)
	defer unlock()
	d, err := s.Get(req.ID)
	if errors.Is(err, ErrNotFound) {
		return ops.ChatDraftResult{Refused: true, Reason: ops.ReasonDraftGone}, nil
	}
	if err != nil {
		return ops.ChatDraftResult{}, err
	}
	if d.Revision != req.Revision {
		return ops.ChatDraftResult{Refused: true, Reason: ops.ReasonDraftRevisionConflict}, nil
	}
	before := *d
	d.Brief = req.Brief
	for dst, src := range map[*string]*string{
		&d.Project: req.Project, &d.Name: req.Name, &d.Harness: req.Harness, &d.Model: req.Model,
		&d.Effort: req.Effort, &d.Folder: req.Folder, &d.Worktree: req.Worktree,
	} {
		if src != nil {
			*dst = *src
		}
	}
	d.Name = strings.TrimSpace(d.Name)
	if check != nil {
		if refusal := check(d, before); refusal != nil {
			return *refusal, nil
		}
	}
	d.Revision++
	d.EditedAt = s.now().UTC()
	if err := s.write(d); err != nil {
		return ops.ChatDraftResult{}, err
	}
	d.Label = Label(d.Name, d.Brief)
	return ops.ChatDraftResult{Draft: d}, nil
}

// Delete removes a draft's folder and reports whether a draft was there.
func (s *Store) Delete(id string) (bool, error) {
	if !idPattern.MatchString(id) {
		return false, nil
	}
	unlock := s.lock(id)
	defer unlock()
	return s.remove(id)
}

// remove is Delete for a caller that already holds the draft's lock.
func (s *Store) remove(id string) (bool, error) {
	if _, err := s.Get(id); errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return true, os.RemoveAll(s.dir(id))
}

// MemberKey is the one name of a chat draft's workspace membership. The
// prefix keeps it from parsing as a Design draft key (see drafts.ParseMemberKey).
func MemberKey(id string) string { return memberPrefix + id }

const memberPrefix = "chat-draft/"

// ParseMemberKey is MemberKey's inverse: the draft id of a member key, and
// false for any other key.
func ParseMemberKey(key string) (id string, ok bool) {
	id, ok = strings.CutPrefix(key, memberPrefix)
	return id, ok && id != ""
}

func (s *Store) write(d *ops.ChatDraft) error {
	rec := *d
	rec.Label = "" // computed on read, never stored
	b, err := json.MarshalIndent(&rec, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.record(d.ID), b)
}

// writeAtomic writes b to path through a temp file in the same folder, so a
// reader never sees a half-written record.
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
