// Package schedules stores Schedules: a prompt plus run times, one markdown
// file per Schedule at <ticket store>/<project dir>/.schedules/<slug>.md. The
// folder is dot-prefixed so every store scan skips it, and nothing
// per-machine (no lock file) lives in it: the store commit loop shares it.
package schedules

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/elentok/gx/fsutil"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/tickets"
)

const (
	// Version is the stored format version.
	Version = 1
	// DefaultLateLimitHours is the Late limit when none is given.
	DefaultLateLimitHours = 2.0

	dirName = ".schedules"
)

// Reasons a Schedule is refused. The server maps them to refusals.
const (
	ReasonEmptyName        = "empty-name"
	ReasonEmptyPrompt      = "empty-prompt"
	ReasonNoDays           = "no-days"
	ReasonNoTimes          = "no-times"
	ReasonInvalidTime      = "invalid-time"
	ReasonInvalidLateLimit = "invalid-late-limit"
	ReasonInvalidHarness   = "invalid-harness"
	ReasonSlugExists       = "slug-exists"
	ReasonInvalidProject   = "invalid-project"
)

// ErrNotFound is returned when a Schedule does not exist.
var ErrNotFound = errors.New("schedule not found")

// Error is a refusal with a stable Reason.
type Error struct {
	Reason string
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Reason
	}
	return e.Reason + ": " + e.Detail
}

// ReasonOf returns the Reason of a refusal, or "" for any other error.
func ReasonOf(err error) string {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Reason
	}
	return ""
}

func refuse(reason, detail string) error { return &Error{Reason: reason, Detail: detail} }

// Schedule is a stored prompt plus its run times. The project is the
// directory it lives in and is never written to the file.
type Schedule struct {
	Name string `yaml:"name"`
	Slug string `yaml:"-"`
	// Project is the project name (tickets.ProjectName of its directory).
	Project string `yaml:"-"`
	Prompt  string `yaml:"-"`
	// Days are weekdays; Times are "HH:MM" local time. Sorted, no duplicates.
	Days  []time.Weekday `yaml:"days"`
	Times []string       `yaml:"times"`
	// Harness is "" (Default, resolved on every run), "claude" or "codex".
	Harness        string    `yaml:"harness,omitempty"`
	LateLimitHours float64   `yaml:"late_limit_hours"`
	CreatedAt      time.Time `yaml:"created_at"`
	UpdatedAt      time.Time `yaml:"updated_at"`
}

// frontmatter is the stored form. Unknown keys are ignored on read, so a later
// "when" form (cron) does not break older readers.
type frontmatter struct {
	Version  int `yaml:"version"`
	Schedule `yaml:",inline"`
}

// Item is one listed Schedule.
type Item struct {
	Schedule
	// Registered reports whether the project directory has a project.json.
	Registered bool
}

var timeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// Validate checks s and normalises it: times and days sorted with duplicates
// collapsed, a zero Late limit defaulted. Project and slug are not checked here.
func Validate(s *Schedule) error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return refuse(ReasonEmptyName, "")
	}
	if strings.TrimSpace(s.Prompt) == "" {
		return refuse(ReasonEmptyPrompt, "")
	}
	if len(s.Days) == 0 {
		return refuse(ReasonNoDays, "")
	}
	for _, d := range s.Days {
		if d < time.Sunday || d > time.Saturday {
			return refuse(ReasonNoDays, fmt.Sprintf("invalid weekday %d", int(d)))
		}
	}
	if len(s.Times) == 0 {
		return refuse(ReasonNoTimes, "")
	}
	for _, tm := range s.Times {
		if !timeRe.MatchString(tm) {
			return refuse(ReasonInvalidTime, tm)
		}
	}
	if s.LateLimitHours == 0 {
		s.LateLimitHours = DefaultLateLimitHours
	}
	if !(s.LateLimitHours > 0) {
		return refuse(ReasonInvalidLateLimit, fmt.Sprint(s.LateLimitHours))
	}
	if err := agent.ValidateAgentKind(agent.AgentKind(s.Harness)); err != nil {
		return refuse(ReasonInvalidHarness, err.Error())
	}
	s.Times = uniqueSorted(s.Times)
	s.Days = uniqueSorted(s.Days)
	return nil
}

func uniqueSorted[T cmp.Ordered](in []T) []T {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}

// Store reads and writes Schedules under one ticket store directory.
type Store struct {
	dir string
	mu  sync.Mutex
	now func() time.Time
}

// NewStore returns a Store over the ticket store directory dir.
func NewStore(dir string) *Store {
	return &Store{dir: dir, now: time.Now}
}

// ProjectNameOf is the project name for a project directory.
func ProjectNameOf(dir string) string { return tickets.ProjectName(dir) }

func (st *Store) path(project, slug string) string {
	return filepath.Join(st.dir, project, dirName, slug+".md")
}

// projectDir resolves a project name to its directory under the store. A
// project is a plain directory name directly under the store.
func (st *Store) projectDir(project string) (string, error) {
	if project == "" || project != filepath.Base(project) || strings.HasPrefix(project, ".") {
		return "", refuse(ReasonInvalidProject, project)
	}
	info, err := os.Stat(filepath.Join(st.dir, project))
	if err != nil || !info.IsDir() {
		return "", refuse(ReasonInvalidProject, project)
	}
	return filepath.Join(st.dir, project), nil
}

// Create validates s and stores it under project (a directory name in the
// store). The slug comes from the name and is fixed from then on.
func (st *Store) Create(project string, s Schedule) (Schedule, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, err := st.projectDir(project); err != nil {
		return Schedule{}, err
	}
	if err := Validate(&s); err != nil {
		return Schedule{}, err
	}
	s.Slug = tickets.Slug(s.Name)
	s.Project = ProjectNameOf(filepath.Join(st.dir, project))
	if _, err := os.Stat(st.path(project, s.Slug)); err == nil {
		return Schedule{}, refuse(ReasonSlugExists, s.Slug)
	}
	now := st.now().UTC()
	s.CreatedAt, s.UpdatedAt = now, now
	return s, st.write(project, s)
}

// Get reads one Schedule.
func (st *Store) Get(project, slug string) (Schedule, error) {
	if _, err := st.projectDir(project); err != nil {
		return Schedule{}, err
	}
	return readFile(st.path(project, slug), slug)
}

// Update replaces the editable fields of an existing Schedule. The slug and
// project never change: s.Project, when set, must equal project.
func (st *Store) Update(project, slug string, s Schedule) (Schedule, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if s.Project != "" && s.Project != project && s.Project != ProjectNameOf(filepath.Join(st.dir, project)) {
		return Schedule{}, refuse(ReasonInvalidProject, "project cannot change")
	}
	old, err := st.Get(project, slug)
	if err != nil {
		return Schedule{}, err
	}
	if err := Validate(&s); err != nil {
		return Schedule{}, err
	}
	s.Slug, s.Project = old.Slug, old.Project
	s.CreatedAt = old.CreatedAt
	s.UpdatedAt = st.now().UTC()
	return s, st.write(project, s)
}

// Delete removes a Schedule.
func (st *Store) Delete(project, slug string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if _, err := st.projectDir(project); err != nil {
		return err
	}
	err := os.Remove(st.path(project, slug))
	if os.IsNotExist(err) {
		return ErrNotFound
	}
	return err
}

// List returns every Schedule in every directory directly under the store,
// registered project or not, ordered by project then slug.
func (st *Store) List() ([]Item, error) {
	entries, err := os.ReadDir(st.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		pdir := filepath.Join(st.dir, e.Name())
		files, err := os.ReadDir(filepath.Join(pdir, dirName))
		if err != nil {
			continue
		}
		_, statErr := os.Stat(filepath.Join(pdir, "project.json"))
		for _, f := range files {
			slug, ok := strings.CutSuffix(f.Name(), ".md")
			if !ok || f.IsDir() {
				continue
			}
			s, err := readFile(filepath.Join(pdir, dirName, f.Name()), slug)
			if err != nil {
				return nil, err
			}
			items = append(items, Item{Schedule: s, Registered: statErr == nil})
		}
	}
	return items, nil
}

func (st *Store) write(project string, s Schedule) error {
	fm, err := yaml.Marshal(frontmatter{Version: Version, Schedule: s})
	if err != nil {
		return err
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(fm)
	b.WriteString("---\n")
	b.WriteString(s.Prompt)
	if !strings.HasSuffix(s.Prompt, "\n") {
		b.WriteString("\n")
	}
	p := st.path(project, s.Slug)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(p, b.Bytes())
}

func readFile(path, slug string) (Schedule, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Schedule{}, ErrNotFound
	}
	if err != nil {
		return Schedule{}, err
	}
	rest, ok := strings.CutPrefix(string(data), "---\n")
	if !ok {
		return Schedule{}, fmt.Errorf("%s: missing frontmatter", path)
	}
	head, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		head, ok = strings.CutSuffix(rest, "\n---")
		if !ok {
			return Schedule{}, fmt.Errorf("%s: unterminated frontmatter", path)
		}
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(head), &fm); err != nil {
		return Schedule{}, fmt.Errorf("%s: %w", path, err)
	}
	s := fm.Schedule
	s.Slug, s.Project = slug, ProjectNameOf(filepath.Dir(filepath.Dir(path)))
	s.Prompt = strings.TrimSuffix(body, "\n")
	return s, nil
}
