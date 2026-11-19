// Package drafthistory keeps the Draft history: three capped lists (Chat
// drafts, Design drafts and Sent messages) in the gx state dir. Each entry owns
// its own copies of the image files.
//
// Layout: <stateDir>/draft-history/<list>/list.json holds the entries newest
// first; <list>/<entry id>/ holds that entry's image files.
package drafthistory

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

// List names one of the three lists.
type List string

const (
	ChatDrafts   List = "chat-drafts"
	DesignDrafts List = "design-drafts"
	Sent         List = "sent"
)

const (
	rootDir  = "draft-history"
	listFile = "list.json"

	// MaxEntries is how many entries one list holds.
	MaxEntries = 50
	// MaxImageBytes is the image total of one list. The store takes the cap as
	// an argument; callers that pass 0 get this.
	MaxImageBytes int64 = 500 << 20
)

// Composer says which composer a Sent messages entry came from.
type Composer string

const (
	ComposerChat   Composer = "chat"
	ComposerDesign Composer = "design"
)

// Entry is one saved text with its settings. Fields that do not apply to the
// entry's list stay empty.
type Entry struct {
	ID      string    `json:"id"`
	SavedAt time.Time `json:"savedAt"`
	Text    string    `json:"text"`
	// Images are the file names kept in the entry's own folder.
	Images []string `json:"images"`
	// ImagesDropped is the "images not kept" mark: the images were too big for the cap.
	ImagesDropped bool `json:"imagesDropped,omitempty"`

	Project string `json:"project,omitempty"`
	// Slug: Design drafts, and Design Sent messages (the epic part of the Ticket key).
	Slug string `json:"slug,omitempty"`
	// Name is the session name: Chat drafts, and chat Sent messages.
	Name     string   `json:"name,omitempty"`
	Harness  string   `json:"harness,omitempty"`
	Model    string   `json:"model,omitempty"`
	Effort   string   `json:"effort,omitempty"`
	Folder   string   `json:"folder,omitempty"`
	Composer Composer `json:"composer,omitempty"`
}

// Item is an entry as List returns it.
type Item struct {
	Entry
	ImageCount int `json:"imageCount"`
}

// Result says what Save did.
type Result int

const (
	Saved Result = iota
	SkippedEmpty
	SkippedRepeat
)

// Store reads and writes the lists under <stateDir>/draft-history.
type Store struct {
	root string
	cap  int64
	now  func() time.Time
}

// listLocks is shared by every Store, so two Stores over one directory
// serialise their writes. It maps a list folder to its *sync.Mutex.
var listLocks sync.Map

// New builds a Store with the default cap and the real clock.
func New(stateDir string) *Store { return NewWith(stateDir, 0, nil) }

// NewWith builds a Store with an image cap (0 means MaxImageBytes) and a clock (nil means time.Now).
func NewWith(stateDir string, capBytes int64, now func() time.Time) *Store {
	if capBytes <= 0 {
		capBytes = MaxImageBytes
	}
	if now == nil {
		now = time.Now
	}
	return &Store{root: filepath.Join(stateDir, rootDir), cap: capBytes, now: now}
}

func (s *Store) dir(l List) string { return filepath.Join(s.root, string(l)) }

func (s *Store) lock(l List) func() {
	m, _ := listLocks.LoadOrStore(s.dir(l), &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (s *Store) read(l List) ([]Entry, error) {
	b, err := os.ReadFile(filepath.Join(s.dir(l), listFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var es []Entry
	if err := json.Unmarshal(b, &es); err != nil {
		return nil, fmt.Errorf("draft history %s: %w", l, err)
	}
	return es, nil
}

func (s *Store) write(l List, es []Entry) error {
	if es == nil {
		es = []Entry{}
	}
	b, err := json.Marshal(es)
	if err != nil {
		return err
	}
	dir := s.dir(l)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(dir, listFile+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, listFile))
}

// List returns the list newest first. Order is the stored order, not the
// clock, so two saves in one tick keep their order.
func (s *Store) List(l List) ([]Item, error) {
	unlock := s.lock(l)
	defer unlock()
	es, err := s.read(l)
	if err != nil {
		return nil, err
	}
	out := make([]Item, len(es))
	for i, e := range es {
		if e.Images == nil {
			e.Images = []string{}
		}
		out[i] = Item{Entry: e, ImageCount: len(e.Images)}
	}
	return out, nil
}

// ImagePaths returns the absolute paths of the image files an entry holds.
func (s *Store) ImagePaths(l List, id string) ([]string, error) {
	unlock := s.lock(l)
	defer unlock()
	es, err := s.read(l)
	if err != nil {
		return nil, err
	}
	for _, e := range es {
		if e.ID == id {
			return s.paths(l, e), nil
		}
	}
	return nil, os.ErrNotExist
}

func (s *Store) paths(l List, e Entry) []string {
	out := make([]string, len(e.Images))
	for i, n := range e.Images {
		out[i] = filepath.Join(s.dir(l), e.ID, n)
	}
	return out
}

// Save adds e to the front of list l, copying the files at imagePaths.
// An entry with no text and no images is skipped as empty. Whitespace-only
// text counts as no text: a draft of blanks is nothing worth recalling.
// An entry equal to the newest one (same text, same settings, byte-identical
// images in the same order) is skipped as a repeat. ID, SavedAt and Images of e
// are ignored.
func (s *Store) Save(l List, e Entry, imagePaths []string) (Result, error) {
	if strings.TrimSpace(e.Text) == "" && len(imagePaths) == 0 {
		return SkippedEmpty, nil
	}
	unlock := s.lock(l)
	defer unlock()
	es, err := s.read(l)
	if err != nil {
		return 0, err
	}
	if len(es) > 0 && s.sameAsNewest(l, es[0], e, imagePaths) {
		return SkippedRepeat, nil
	}

	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	e.ID = hex.EncodeToString(raw[:])
	e.SavedAt = s.now().UTC()
	e.Images = []string{}
	e.ImagesDropped = false
	if err := s.copyImages(l, &e, imagePaths); err != nil {
		_ = os.RemoveAll(filepath.Join(s.dir(l), e.ID))
		return 0, err
	}

	es = append([]Entry{e}, es...)
	var drop []Entry
	if len(es) > MaxEntries {
		drop = append(drop, es[MaxEntries:]...)
		es = es[:MaxEntries]
	}
	// Drop the oldest until the image bytes fit. The new entry (index 0) stays.
	total := s.imageBytes(l, es)
	for total > s.cap && len(es) > 1 {
		last := es[len(es)-1]
		total -= s.imageBytes(l, []Entry{last})
		drop = append(drop, last)
		es = es[:len(es)-1]
	}
	if err := s.write(l, es); err != nil {
		_ = os.RemoveAll(filepath.Join(s.dir(l), e.ID))
		return 0, err
	}
	for _, d := range drop {
		_ = os.RemoveAll(filepath.Join(s.dir(l), d.ID))
	}
	return Saved, nil
}

// copyImages copies the files into the entry's folder. When they alone pass
// the cap, none is kept and the entry gets the "images not kept" mark.
func (s *Store) copyImages(l List, e *Entry, src []string) error {
	if len(src) == 0 {
		return nil
	}
	var total int64
	for _, p := range src {
		fi, err := os.Stat(p)
		if err != nil {
			return err
		}
		total += fi.Size()
	}
	if total > s.cap {
		e.ImagesDropped = true
		return nil
	}
	dir := filepath.Join(s.dir(l), e.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, p := range src {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		name := filepath.Base(p)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			return err
		}
		e.Images = append(e.Images, name)
	}
	return nil
}

func (s *Store) imageBytes(l List, es []Entry) int64 {
	var n int64
	for _, e := range es {
		for _, p := range s.paths(l, e) {
			if fi, err := os.Stat(p); err == nil {
				n += fi.Size()
			}
		}
	}
	return n
}

func (s *Store) sameAsNewest(l List, newest, e Entry, imagePaths []string) bool {
	a, b := newest, e
	a.ID, b.ID = "", ""
	a.SavedAt, b.SavedAt = time.Time{}, time.Time{}
	a.Images, b.Images = nil, nil
	a.ImagesDropped = false
	if !reflect.DeepEqual(a, b) || len(newest.Images) != len(imagePaths) {
		return false
	}
	held := s.paths(l, newest)
	for i, p := range imagePaths {
		x, err1 := os.ReadFile(p)
		y, err2 := os.ReadFile(held[i])
		if err1 != nil || err2 != nil || !bytes.Equal(x, y) {
			return false
		}
	}
	return true
}
