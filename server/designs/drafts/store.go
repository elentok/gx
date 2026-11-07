// Package drafts keeps New Design drafts: one JSON file per draft in its own
// folder of the gx state dir. A draft is not a map epic and never shows as one.
package drafts

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/wire"
)

const (
	rootDir     = "design-drafts"
	recordName  = "draft.json"
	lastUsedRec = "last-used.json"
)

// ErrNotFound: the draft id does not exist (or is not a well-formed id).
var ErrNotFound = errors.New("draft not found")

var idPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Store reads and writes drafts under <stateDir>/design-drafts. Each draft
// lives in <id>/draft.json, with its images in <id>/images (written by the
// image ops; the store reaches that folder by path only).
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

// ImagesDir is the folder a draft's images live in.
func (s *Store) ImagesDir(id string) string { return filepath.Join(s.root, id, "images") }

func (s *Store) dir(id string) string    { return filepath.Join(s.root, id) }
func (s *Store) record(id string) string { return filepath.Join(s.dir(id), recordName) }

func (s *Store) lock(id string) func() {
	m, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func isEmpty(d *wire.DesignDraft) bool {
	return d.Name == "" && d.Slug == "" && d.Idea == "" && len(d.Images) == 0
}

// Create stores a new draft at revision 1. A draft with nothing in it stores
// nothing and returns nil.
func (s *Store) Create(project, slug, idea, name string) (*wire.DesignDraft, error) {
	d := &wire.DesignDraft{Project: project, Slug: slug, Idea: idea, Name: name, Images: []string{}}
	if isEmpty(d) {
		return nil, nil
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, err
	}
	d.ID = hex.EncodeToString(raw[:])
	d.Revision = 1
	d.EditedAt = s.now().UTC()
	unlock := s.lock(d.ID)
	defer unlock()
	if err := s.write(d); err != nil {
		return nil, err
	}
	return d, nil
}

// Get reads a draft without changing its revision or edited-at.
func (s *Store) Get(id string) (*wire.DesignDraft, error) {
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
	var d wire.DesignDraft
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("draft %s: %w", id, err)
	}
	if d.Images == nil {
		d.Images = []string{}
	}
	return &d, nil
}

// List returns every draft, most recently edited first. An unreadable draft is skipped.
func (s *Store) List() ([]wire.DesignDraftItem, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return []wire.DesignDraftItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := []wire.DesignDraftItem{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		d, err := s.Get(e.Name())
		if err != nil {
			continue
		}
		items = append(items, wire.DesignDraftItem{
			ID: d.ID, Project: d.Project, Slug: d.Slug, Idea: d.Idea, Name: d.Name,
			ImageCount: len(d.Images), EditedAt: d.EditedAt,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].EditedAt.After(items[j].EditedAt) })
	return items, nil
}

// Save applies a text save. It never touches the image list. A stale revision
// or a missing draft is a refused result, not an error. A save that leaves the
// draft empty deletes it and reports Deleted.
func (s *Store) Save(id string, req wire.DesignDraftSaveRequest) (wire.DesignDraftSaveResult, error) {
	if !idPattern.MatchString(id) {
		return wire.DesignDraftSaveResult{Refused: true, Reason: wire.ReasonDraftGone}, nil
	}
	unlock := s.lock(id)
	defer unlock()
	d, err := s.Get(id)
	if errors.Is(err, ErrNotFound) {
		return wire.DesignDraftSaveResult{Refused: true, Reason: wire.ReasonDraftGone}, nil
	}
	if err != nil {
		return wire.DesignDraftSaveResult{}, err
	}
	if d.Revision != req.Revision {
		return wire.DesignDraftSaveResult{Refused: true, Reason: wire.ReasonDraftRevisionConflict}, nil
	}
	d.Project, d.Slug, d.Idea, d.Name = req.Project, req.Slug, req.Idea, req.Name
	if isEmpty(d) {
		if err := os.RemoveAll(s.dir(id)); err != nil {
			return wire.DesignDraftSaveResult{}, err
		}
		return wire.DesignDraftSaveResult{Deleted: true}, nil
	}
	d.Revision++
	if !req.KeepEditedAt {
		d.EditedAt = s.now().UTC()
	}
	if err := s.write(d); err != nil {
		return wire.DesignDraftSaveResult{}, err
	}
	return wire.DesignDraftSaveResult{Draft: d}, nil
}

func goneImage() wire.SessionImageResult {
	return imagestore.Refusal(wire.ReasonDraftGone, "draft is gone")
}

// AddImage stores one image in the draft's folder and lists its id on the
// record. It changes the image list only: the text revision stays, so a text
// save in flight never conflicts with it. A missing draft is a refused result.
func (s *Store) AddImage(id string, data []byte) (wire.SessionImageResult, error) {
	if !idPattern.MatchString(id) {
		return goneImage(), nil
	}
	unlock := s.lock(id)
	defer unlock()
	d, err := s.Get(id)
	if errors.Is(err, ErrNotFound) {
		return goneImage(), nil
	}
	if err != nil {
		return wire.SessionImageResult{}, err
	}
	if len(d.Images) >= wire.MaxPromptImages {
		return imagestore.Refusal(wire.ReasonImageTooMany, fmt.Sprintf("a draft holds at most %d images", wire.MaxPromptImages)), nil
	}
	res, err := imagestore.Save(s.ImagesDir(id), data)
	if res.Refused || err != nil {
		return res, err
	}
	d.Images = append(d.Images, res.ID)
	if err := s.write(d); err != nil {
		_, _, _ = imagestore.Remove(s.ImagesDir(id), res.ID)
		return wire.SessionImageResult{}, err
	}
	return res, nil
}

// RemoveImage deletes one image file and drops its id from the record, leaving
// the text revision alone.
func (s *Store) RemoveImage(id, image string) (wire.SessionImageResult, error) {
	if !idPattern.MatchString(id) {
		return goneImage(), nil
	}
	unlock := s.lock(id)
	defer unlock()
	d, err := s.Get(id)
	if errors.Is(err, ErrNotFound) {
		return goneImage(), nil
	}
	if err != nil {
		return wire.SessionImageResult{}, err
	}
	i := slices.Index(d.Images, image)
	if i < 0 {
		return imagestore.Refusal(wire.ReasonImageNotFound, fmt.Sprintf("no image %s in draft %s", image, id)), nil
	}
	name, _, err := imagestore.Remove(s.ImagesDir(id), image)
	if err != nil {
		return wire.SessionImageResult{}, err
	}
	d.Images = slices.Delete(d.Images, i, i+1)
	if err := s.write(d); err != nil {
		return wire.SessionImageResult{}, err
	}
	return wire.SessionImageResult{ID: image, Name: name}, nil
}

// Delete removes a draft's record and its image folder. A missing draft is not an error.
func (s *Store) Delete(id string) error {
	if !idPattern.MatchString(id) {
		return nil
	}
	unlock := s.lock(id)
	defer unlock()
	return os.RemoveAll(s.dir(id))
}

// LastProject is the last-used project name, "" when none was ever set.
func (s *Store) LastProject() (string, error) {
	b, err := os.ReadFile(filepath.Join(s.root, lastUsedRec))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var rec struct {
		Project string `json:"project"`
	}
	if err := json.Unmarshal(b, &rec); err != nil {
		return "", nil
	}
	return rec.Project, nil
}

// SetLastProject records the last-used project; the newest write wins.
func (s *Store) SetLastProject(project string) error {
	b, err := json.Marshal(struct {
		Project string `json:"project"`
	}{strings.TrimSpace(project)})
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.root, lastUsedRec), b)
}

func (s *Store) write(d *wire.DesignDraft) error {
	b, err := json.MarshalIndent(d, "", "  ")
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
