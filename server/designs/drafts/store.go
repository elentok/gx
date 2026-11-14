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
	"github.com/elentok/gx/server/wire/ops"
)

const (
	rootDir     = "design-drafts"
	recordName  = "draft.json"
	lastUsedRec = "last-used.json"
)

// ErrNotFound: the draft id does not exist (or is not a well-formed id).
var ErrNotFound = errors.New("draft not found")

var idPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// SlugRe is the one slug rule: lowercase words joined by "-". server/designs
// checks a design's slug with it too.
var SlugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Store reads and writes drafts under <stateDir>/design-drafts. Each draft
// lives in <id>/draft.json, with its images in <id>/images (written by the
// image ops; the store reaches that folder by path only).
type Store struct {
	root string
	now  func() time.Time
	// locks maps a draft id to its *sync.Mutex.
	locks sync.Map
	// leases maps a draft id to when its page lease ends; guarded by leaseMu.
	leaseMu sync.Mutex
	leases  map[string]time.Time
}

const (
	// LeasePing is how often an open draft page pings its lease.
	LeasePing = 30 * time.Second
	// LeaseTTL is how long one ping keeps a draft from the sweep: a few missed pings.
	LeaseTTL = 2 * time.Minute
)

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

// isEmpty: no prompt, no images and no slug. The project never counts.
func isEmpty(d *ops.DesignDraft) bool {
	return d.Slug == "" && d.Idea == "" && len(d.Images) == 0
}

const (
	labelWords    = 5
	labelMaxRunes = 40
	labelFallback = "Untitled design"
)

// Label is the one rule for a draft's display name: the slug, else the first
// 5 words of the prompt cut at 40 characters, else "Untitled design".
func Label(slug, prompt string) string {
	if slug != "" {
		return slug
	}
	words := strings.Fields(prompt)
	if len(words) == 0 {
		return labelFallback
	}
	if len(words) > labelWords {
		words = words[:labelWords]
	}
	r := []rune(strings.Join(words, " "))
	return string(r[:min(len(r), labelMaxRunes)])
}

// Create stores a new draft at revision 1. An empty draft is kept.
func (s *Store) Create(project, slug, idea string) (*ops.DesignDraft, error) {
	d := &ops.DesignDraft{Project: project, Slug: slug, Idea: idea, Images: []string{}}
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
	d.Label = Label(d.Slug, d.Idea)
	return d, nil
}

// Get reads a draft without changing its revision or edited-at.
func (s *Store) Get(id string) (*ops.DesignDraft, error) {
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
	var d ops.DesignDraft
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("draft %s: %w", id, err)
	}
	if d.Images == nil {
		d.Images = []string{}
	}
	d.Label = Label(d.Slug, d.Idea)
	return &d, nil
}

// List returns every draft, most recently edited first. An unreadable draft is skipped.
func (s *Store) List() ([]ops.DesignDraftItem, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return []ops.DesignDraftItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := []ops.DesignDraftItem{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		d, err := s.Get(e.Name())
		if err != nil {
			continue
		}
		items = append(items, ops.DesignDraftItem{
			ID: d.ID, Project: d.Project, Slug: d.Slug, Idea: d.Idea, Label: d.Label,
			ImageCount: len(d.Images), EditedAt: d.EditedAt,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].EditedAt.After(items[j].EditedAt) })
	return items, nil
}

// Save applies a text save. It never touches the image list. A stale revision
// or a missing draft is a refused result, not an error. A save that leaves the
// draft empty deletes it and reports Deleted.
func (s *Store) Save(id string, req ops.DesignDraftSaveRequest) (ops.DesignDraftSaveResult, error) {
	if !idPattern.MatchString(id) {
		return ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftGone}, nil
	}
	unlock := s.lock(id)
	defer unlock()
	d, err := s.Get(id)
	if errors.Is(err, ErrNotFound) {
		return ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftGone}, nil
	}
	if err != nil {
		return ops.DesignDraftSaveResult{}, err
	}
	if d.Revision != req.Revision {
		return ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftRevisionConflict}, nil
	}
	d.Idea = req.Idea
	d.Revision++
	if !req.KeepEditedAt {
		d.EditedAt = s.now().UTC()
	}
	if err := s.write(d); err != nil {
		return ops.DesignDraftSaveResult{}, err
	}
	d.Label = Label(d.Slug, d.Idea)
	return ops.DesignDraftSaveResult{Draft: d}, nil
}

// SetTarget writes the slug and/or the project under the draft's lock; a nil
// field is left alone. It never changes Revision, so a text save in flight
// does not conflict with it, and it moves EditedAt unless keepEditedAt. An
// empty slug clears it. A non-empty slug failing SlugRe and a blank project
// are refused and nothing is written. A missing draft is a refused result.
func (s *Store) SetTarget(id string, project, slug *string, keepEditedAt bool) (ops.DesignDraftSaveResult, error) {
	if slug != nil && *slug != "" && !SlugRe.MatchString(*slug) {
		return ops.DesignDraftSaveResult{Refused: true, Reason: fmt.Sprintf("%q is not a slug (want lowercase words joined by -)", *slug)}, nil
	}
	if project != nil && strings.TrimSpace(*project) == "" {
		return ops.DesignDraftSaveResult{Refused: true, Reason: "project must not be blank"}, nil
	}
	if !idPattern.MatchString(id) {
		return ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftGone}, nil
	}
	unlock := s.lock(id)
	defer unlock()
	d, err := s.Get(id)
	if errors.Is(err, ErrNotFound) {
		return ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftGone}, nil
	}
	if err != nil {
		return ops.DesignDraftSaveResult{}, err
	}
	if project != nil {
		d.Project = *project
	}
	if slug != nil {
		d.Slug = *slug
	}
	if !keepEditedAt {
		d.EditedAt = s.now().UTC()
	}
	if err := s.write(d); err != nil {
		return ops.DesignDraftSaveResult{}, err
	}
	d.Label = Label(d.Slug, d.Idea)
	return ops.DesignDraftSaveResult{Draft: d}, nil
}

// MigrateNames folds the retired draft name into the record: it becomes the
// slug when the slug is empty and the slugified name is a valid slug, else the
// first line of the prompt. The name key is then dropped, so a second run
// changes nothing. It runs once at server start, before any sweep.
func (s *Store) MigrateNames() error {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := s.migrateName(e.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) migrateName(id string) error {
	if !idPattern.MatchString(id) {
		return nil
	}
	unlock := s.lock(id)
	defer unlock()
	b, err := os.ReadFile(s.record(id))
	if err != nil {
		return nil // unreadable or missing records are List's problem, not the migration's
	}
	var rec map[string]any
	if json.Unmarshal(b, &rec) != nil {
		return nil
	}
	raw, has := rec["name"]
	if !has {
		return nil
	}
	delete(rec, "name")
	name, _ := raw.(string)
	if name = strings.TrimSpace(name); name != "" {
		slug, _ := rec["slug"].(string)
		if cand := slugify(name); slug == "" && SlugRe.MatchString(cand) {
			rec["slug"] = cand
		} else {
			idea, _ := rec["idea"].(string)
			if idea != "" {
				name += "\n" + idea
			}
			rec["idea"] = name
		}
	}
	out, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.record(id), out)
}

var nonSlugRun = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	return strings.Trim(nonSlugRun.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

func goneImage() wire.SessionImageResult {
	return imagestore.Refusal(ops.ReasonDraftGone, "draft is gone")
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

// DeleteIfEmpty deletes a draft only while it is still empty, judged under the
// draft's lock on a fresh read, so text saved elsewhere in the meantime keeps
// it. Deleted is set when it removed the draft; otherwise the result is refused
// draft-gone or draft-not-empty.
func (s *Store) DeleteIfEmpty(id string) (ops.DesignDraftSaveResult, error) {
	if !idPattern.MatchString(id) {
		return ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftGone}, nil
	}
	unlock := s.lock(id)
	defer unlock()
	d, err := s.Get(id)
	if errors.Is(err, ErrNotFound) {
		return ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftGone}, nil
	}
	if err != nil {
		return ops.DesignDraftSaveResult{}, err
	}
	if !isEmpty(d) {
		return ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftNotEmpty}, nil
	}
	if err := os.RemoveAll(s.dir(id)); err != nil {
		return ops.DesignDraftSaveResult{}, err
	}
	return ops.DesignDraftSaveResult{Deleted: true}, nil
}

// Lease records that a draft's page is open until now+LeaseTTL. Leases live in
// memory only: after a server start none is live. A missing draft is refused
// draft-gone.
func (s *Store) Lease(id string) (ops.DesignDraftSaveResult, error) {
	if _, err := s.Get(id); errors.Is(err, ErrNotFound) {
		return ops.DesignDraftSaveResult{Refused: true, Reason: ops.ReasonDraftGone}, nil
	} else if err != nil {
		return ops.DesignDraftSaveResult{}, err
	}
	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()
	if s.leases == nil {
		s.leases = map[string]time.Time{}
	}
	s.leases[id] = s.now().Add(LeaseTTL)
	return ops.DesignDraftSaveResult{}, nil
}

func (s *Store) leased(id string) bool {
	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()
	return s.now().Before(s.leases[id])
}

// SweepEmpty deletes every empty draft that has no live lease, except the ids
// in skip, and returns the ids it removed. It deletes through DeleteIfEmpty, so
// text that arrived meanwhile still keeps a draft.
func (s *Store) SweepEmpty(skip ...string) ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, e := range entries {
		id := e.Name()
		if !e.IsDir() || !idPattern.MatchString(id) || slices.Contains(skip, id) || s.leased(id) {
			continue
		}
		res, err := s.DeleteIfEmpty(id)
		if err != nil {
			return removed, err
		}
		if res.Deleted {
			removed = append(removed, id)
		}
	}
	return removed, nil
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

// startRecordTTL is how long a Start record keeps an old draft URL redirecting.
const startRecordTTL = 7 * 24 * time.Hour

const startsDir = "starts"

// MemberKey is the one name of a draft's workspace membership. The "/" keeps it
// from parsing as an epic address, and it is never a session id.
func MemberKey(id string) string { return memberPrefix + id }

const memberPrefix = "draft/"

// ParseMemberKey is MemberKey's inverse: the draft id of a member key, and
// false for any other key.
func ParseMemberKey(key string) (id string, ok bool) {
	id, ok = strings.CutPrefix(key, memberPrefix)
	return id, ok && id != ""
}

type startRecord struct {
	Design    string    `json:"design"`
	StartedAt time.Time `json:"startedAt"`
}

func (s *Store) startPath(id string) string {
	return filepath.Join(s.root, startsDir, id+".json")
}

// RecordStart remembers that draft id became the Design at designAddr
// ("project:epic"), so the old draft URL can redirect to it.
func (s *Store) RecordStart(id, designAddr string) error {
	if !idPattern.MatchString(id) {
		return nil
	}
	b, err := json.Marshal(startRecord{Design: designAddr, StartedAt: s.now().UTC()})
	if err != nil {
		return err
	}
	if err := writeAtomic(s.startPath(id), b); err != nil {
		return err
	}
	s.pruneStarts()
	return nil
}

// Started is the Design address draft id became, "" when no live record exists.
func (s *Store) Started(id string) string {
	if !idPattern.MatchString(id) {
		return ""
	}
	s.pruneStarts()
	b, err := os.ReadFile(s.startPath(id))
	if err != nil {
		return ""
	}
	var rec startRecord
	if json.Unmarshal(b, &rec) != nil {
		return ""
	}
	return rec.Design
}

// pruneStarts deletes records older than startRecordTTL. Failures are ignored:
// a stale record only keeps a redirect a little longer.
func (s *Store) pruneStarts() {
	dir := filepath.Join(s.root, startsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := s.now().Add(-startRecordTTL)
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var rec startRecord
		if json.Unmarshal(b, &rec) != nil || rec.StartedAt.Before(cutoff) {
			_ = os.Remove(p)
		}
	}
}

func (s *Store) write(d *ops.DesignDraft) error {
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
