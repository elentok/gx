// Package triage stores the durable state of a Triage: one JSON record per
// Triage in the project's ticket store, under <project dir>/.triage/. The
// folder is dot-prefixed so the epic loader skips it, and a missing folder
// reads as "no Triage".
package triage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/elentok/gx/fsutil"
	"github.com/elentok/gx/tickets"
)

const (
	// Version is the record format version. Load reads versions 1 and 2.
	Version = 2

	dirName    = ".triage"
	markerName = "live"
	lockName   = ".lock"
)

// State is where a Triage is in its life.
type State string

const (
	StateLive      State = "live"
	StateDone      State = "done"
	StateAbandoned State = "abandoned"
)

// WriteState is how far writing one row got.
type WriteState string

const (
	WritePending WriteState = "pending"
	WriteDone    WriteState = "done"
	WriteFailed  WriteState = "failed"
	WriteGone    WriteState = "gone"
)

// QuestionKind says who settles a question: the agent (decide), the person
// (ask), or the design that follows the triage (design).
type QuestionKind string

const (
	QuestionDecide QuestionKind = "decide"
	QuestionAsk    QuestionKind = "ask"
	QuestionDesign QuestionKind = "design"

	// legacyQuick is the version 1 name of ask; Load reads it as ask.
	legacyQuick QuestionKind = "quick"
)

// Question is one question about a row. Options and Default (the recommended
// answer) are set on every question.
type Question struct {
	ID      string       `json:"id"`
	RowID   string       `json:"rowId,omitempty"`
	Text    string       `json:"text"`
	Kind    QuestionKind `json:"kind"`
	Options []string     `json:"options,omitempty"`
	Default string       `json:"default,omitempty"`
}

// Round is one saved set of questions with the person's answers. An objection
// round is outside the 3-round cap.
type Round struct {
	Questions []Question        `json:"questions"`
	Answers   map[string]string `json:"answers,omitempty"`
	Answered  bool              `json:"answered,omitempty"`
	Objection bool              `json:"objection,omitempty"`
}

// Decided is a question the agent settled itself, with the short reason.
type Decided struct {
	RowID    string `json:"rowId,omitempty"`
	Question string `json:"question"`
	Choice   string `json:"choice"`
	Reason   string `json:"reason,omitempty"`
	Objected bool   `json:"objected,omitempty"`
}

// Area sources.
const (
	AreaTable = "table"
	AreaAgent = "agent"
)

// Area is a map epic area a row can go to.
type Area struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	Confirmed bool   `json:"confirmed,omitempty"`
	Created   bool   `json:"created,omitempty"`
}

// MoveState is how far moving one old epic got.
type MoveState string

const (
	MovePending MoveState = "pending"
	MoveDone    MoveState = "done"
	MoveFailed  MoveState = "failed"
	MoveKept    MoveState = "kept"
)

// MoveUnit moves one old epic into a target area.
type MoveUnit struct {
	OldEpic   string    `json:"oldEpic"`
	Area      string    `json:"area"`
	State     MoveState `json:"state"`
	LastError string    `json:"lastError,omitempty"`
}

// Answered is a question the person answered in an earlier round, as asked.
type Answered struct {
	Text   string `json:"text"`
	Answer string `json:"answer"`
}

// PlanRow is the agent's proposed action for a row.
type PlanRow struct {
	RowID       string     `json:"rowId"`
	Action      string     `json:"action"`
	Destination string     `json:"destination,omitempty"`
	Questions   []Question `json:"questions,omitempty"`
	OverlapWith []string   `json:"overlapWith,omitempty"`
	// Estimate is the row's size estimate for a ready row, such as "12K".
	Estimate string `json:"estimate,omitempty"`
	// Fallback is the area a ready row goes to when its own area is unusable.
	Fallback string `json:"fallback,omitempty"`
	// Investigate marks an area row that needs investigation but no decision:
	// it is written as a research ticket.
	Investigate bool `json:"investigate,omitempty"`
}

// PersonFields are what the person decided for a row.
type PersonFields struct {
	Action       string            `json:"action,omitempty"`
	Destination  string            `json:"destination,omitempty"`
	Answers      map[string]string `json:"answers,omitempty"`
	KeepForLater bool              `json:"keepForLater,omitempty"`
}

// Row is one follow-up in the snapshot, with everything the Triage learned
// about it.
type Row struct {
	Address  string `json:"address"`
	Title    string `json:"title"`
	Category string `json:"category,omitempty"`

	Verdict              string     `json:"verdict,omitempty"`
	Evidence             string     `json:"evidence,omitempty"`
	Files                []string   `json:"files,omitempty"`
	Questions            []Question `json:"questions,omitempty"`
	SuggestedDestination string     `json:"suggestedDestination,omitempty"`
	// Fallback is the area a ready row goes to when its own area is unusable.
	Fallback string       `json:"fallback,omitempty"`
	Plan     *PlanRow     `json:"plan,omitempty"`
	Person   PersonFields `json:"person"`
	Write    WriteState   `json:"write,omitempty"`
	// NewAddresses are the tickets the row became. NewAddress is the single
	// address an older record stored; Load moves it into the list.
	NewAddresses []string `json:"newAddresses,omitempty"`
	NewAddress   string   `json:"newAddress,omitempty"`
	// Group marks the rows that were promoted into one ticket group.
	Group string `json:"group,omitempty"`
	// Refusal is the text of the last refusal the write hit.
	Refusal string `json:"refusal,omitempty"`
}

// Record is one Triage.
type Record struct {
	Version   int        `json:"version"`
	ID        string     `json:"id"`
	StartedAt time.Time  `json:"startedAt"`
	Project   string     `json:"project"`
	State     State      `json:"state"`
	Step      string     `json:"step"`
	Revision  int        `json:"revision"`
	Rows      []Row      `json:"rows"`
	Rounds    []Round    `json:"rounds,omitempty"`
	Decided   []Decided  `json:"decided,omitempty"`
	Areas     []Area     `json:"areas,omitempty"`
	Moves     []MoveUnit `json:"moves,omitempty"`
	// Approved is set by the approve verb.
	Approved bool `json:"approved,omitempty"`
	// storedVersion is the version the record had on disk; zero for a record
	// built in memory.
	storedVersion int
	// LastPassComplete is set when the last verify pass covered every row.
	LastPassComplete bool   `json:"lastPassComplete,omitempty"`
	Result           string `json:"result,omitempty"`
	// ReadyEpic and MapEpics are the epics this Triage creates, recorded before
	// each `epic add` runs so a crash between the two never fails the rerun.
	ReadyEpic string   `json:"readyEpic,omitempty"`
	MapEpics  []string `json:"mapEpics,omitempty"`
}

// ListsEpic reports whether the record names epic as one it creates.
func (r *Record) ListsEpic(epic string) bool {
	return epic != "" && (r.ReadyEpic == epic || slices.Contains(r.MapEpics, epic) ||
		slices.ContainsFunc(r.Areas, func(a Area) bool { return a.Created && a.Name == epic }))
}

// LiveError is returned by Create when the project already has a live Triage.
type LiveError struct{ ID string }

func (e *LiveError) Error() string { return "a Triage is already live: " + e.ID }

// ErrStaleRevision is returned by Save when the record changed since it was read.
var ErrStaleRevision = errors.New("triage record changed since it was read")

// Store is the .triage folder of one project.
type Store struct {
	projectDir string
}

// NewStore returns the store of projectDir; nothing is created until Create.
func NewStore(projectDir string) *Store { return &Store{projectDir: projectDir} }

// ProjectDir is the project's ticket directory the store lives under.
func (s *Store) ProjectDir() string { return s.projectDir }

func (s *Store) dir() string { return filepath.Join(s.projectDir, dirName) }

func (s *Store) recordPath(id string) string { return filepath.Join(s.dir(), id+".json") }

func (s *Store) markerPath() string { return filepath.Join(s.dir(), markerName) }

// lock serializes every writer of the folder across processes.
func (s *Store) lock() (unlock func(), err error) {
	if err := os.MkdirAll(s.dir(), 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(s.dir(), lockName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { f.Close() }, nil
		}
		if err != syscall.EWOULDBLOCK || time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("acquiring triage lock at %s: %w", path, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func newID(now time.Time) (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b[:]), nil
}

// snapshot lists the draft and open follow-ups of the project.
func (s *Store) snapshot() []Row {
	epic := tickets.LoadEpicAt(filepath.Join(s.projectDir, tickets.FollowUpsEpic))
	project := tickets.ProjectName(s.projectDir)
	rows := []Row{}
	for _, t := range epic.Tickets {
		switch strings.ToLower(strings.TrimSpace(t.Status)) {
		case "draft", "open":
			rows = append(rows, Row{
				Address:  project + ":" + tickets.FollowUpsEpic + "/" + t.Identifier,
				Title:    t.Title,
				Category: t.Category,
				Write:    WritePending,
			})
		}
	}
	return rows
}

// HasFollowUps reports whether the project has a draft or open follow-up.
func (s *Store) HasFollowUps() bool { return len(s.snapshot()) > 0 }

// Create starts a Triage at the verify step. It refuses with *LiveError when
// the project already has a live one. A live marker whose record is missing
// (a crash between the two writes) is stale and replaced.
func (s *Store) Create() (*Record, error) {
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	// Under the lock no other creator is mid-write, so a marker naming no
	// live record can only be a crash leftover.
	if raw, err := os.ReadFile(s.markerPath()); err == nil {
		id := strings.TrimSpace(string(raw))
		if rec, err := s.Load(id); err == nil && rec != nil && rec.State == StateLive {
			return nil, &LiveError{ID: id}
		}
		os.Remove(s.markerPath())
	}

	now := time.Now()
	id, err := newID(now)
	if err != nil {
		return nil, err
	}
	marker, err := os.OpenFile(s.markerPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	_, werr := marker.WriteString(id)
	marker.Close()
	if werr != nil {
		os.Remove(s.markerPath())
		return nil, werr
	}
	rec := &Record{
		Version:   Version,
		ID:        id,
		StartedAt: now.UTC(),
		Project:   tickets.ProjectName(s.projectDir),
		State:     StateLive,
		Step:      tickets.StepVerify,
		Revision:  1,
		Rows:      s.snapshot(),
	}
	if err := s.write(rec); err != nil {
		os.Remove(s.markerPath())
		return nil, err
	}
	return rec, nil
}

func (s *Store) write(rec *Record) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(s.recordPath(rec.ID), data)
}

// Save writes rec, which must carry the revision it was read at. A stale
// revision returns ErrStaleRevision and leaves the stored record unchanged.
// On success rec.Revision is bumped. A record leaving live releases the marker.
func (s *Store) Save(rec *Record) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return s.saveLocked(rec)
}

func (s *Store) saveLocked(rec *Record) error {
	cur, err := s.Load(rec.ID)
	if err != nil {
		return err
	}
	if cur == nil {
		return fmt.Errorf("triage %s not found", rec.ID)
	}
	if cur.Revision != rec.Revision {
		return ErrStaleRevision
	}
	next := *rec
	next.Version = Version
	next.Revision++
	if err := s.write(&next); err != nil {
		return err
	}
	rec.Revision = next.Revision
	if rec.State != StateLive {
		if raw, err := os.ReadFile(s.markerPath()); err == nil && strings.TrimSpace(string(raw)) == rec.ID {
			os.Remove(s.markerPath())
		}
	}
	return nil
}

// Load returns the record with id, or nil when there is none.
func (s *Store) Load(id string) (*Record, error) {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return nil, nil
	}
	raw, err := os.ReadFile(s.recordPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, fmt.Errorf("reading triage %s: %w", id, err)
	}
	for i := range rec.Rows {
		r := &rec.Rows[i]
		if r.NewAddress != "" && len(r.NewAddresses) == 0 {
			r.NewAddresses = []string{r.NewAddress}
		}
		r.NewAddress = ""
	}
	var earlier [][]Answered
	if rec.Version < Version {
		var legacy struct {
			Rows []struct {
				Earlier []Answered `json:"earlier"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return nil, fmt.Errorf("reading triage %s: %w", id, err)
		}
		for _, r := range legacy.Rows {
			earlier = append(earlier, r.Earlier)
		}
	}
	rec.upgrade(earlier)
	return &rec, nil
}

// upgrade lifts a version 1 record to the version 2 shape in memory. Nothing
// is written until the next Save. earlier holds each row's version 1
// "earlier" answers, which Row no longer has a field for.
func (rec *Record) upgrade(earlier [][]Answered) {
	rec.storedVersion = rec.Version
	if rec.Version >= Version {
		return
	}
	rec.Version = Version
	for i := range rec.Rows {
		row := &rec.Rows[i]
		for j := range row.Questions {
			q := &row.Questions[j]
			if q.Kind == legacyQuick {
				q.Kind = QuestionAsk
			}
		}
		if row.Plan != nil && row.Plan.Action == legacyMap {
			row.Plan.Action = ActionArea
		}
		if row.Person.Action == legacyMap {
			row.Person.Action = ActionArea
		}
	}
	if rec.State != StateLive {
		return
	}
	switch rec.Step {
	case tickets.StepPlan, tickets.StepApprove:
		rec.backToRoute(earlier)
	}
}

// LegacyWrite reports whether the record is a version 1 record that was at
// write when loaded. Every write-step operation refuses it; only Abandon works.
func (rec *Record) LegacyWrite() bool {
	return rec.State == StateLive && rec.Step == tickets.StepWrite &&
		rec.storedVersion > 0 && rec.storedVersion < Version
}

// backToRoute moves a version 1 record at plan or approve to route. Verdicts
// stay, each given answer becomes an answered ask question, and the plan and
// the person's edits are dropped.
func (rec *Record) backToRoute(earlier [][]Answered) {
	round := Round{Answers: map[string]string{}, Answered: true}
	for i := range rec.Rows {
		row := &rec.Rows[i]
		var open []Question
		for _, q := range row.AllQuestions() {
			q.RowID = row.Address
			if q.Kind == legacyQuick {
				q.Kind = QuestionAsk
			}
			a := strings.TrimSpace(row.Person.Answers[q.ID])
			if a == "" || q.Kind == QuestionDesign {
				open = append(open, q)
				continue
			}
			round.Questions = append(round.Questions, q)
			round.Answers[q.ID] = a
		}
		var rowEarlier []Answered
		if i < len(earlier) {
			rowEarlier = earlier[i]
		}
		for k, e := range rowEarlier {
			id := fmt.Sprintf("earlier-%d", len(round.Questions)+k+1)
			round.Questions = append(round.Questions, Question{ID: id, RowID: row.Address, Text: e.Text, Kind: QuestionAsk, Options: []string{e.Answer}, Default: e.Answer})
			round.Answers[id] = e.Answer
		}
		row.Questions = open
		row.Plan = nil
		row.Person = PersonFields{}
	}
	if len(round.Questions) > 0 {
		rec.Rounds = append(rec.Rounds, round)
	}
	rec.Step = tickets.StepRoute
}

// LoadLive returns the project's live record, or nil when there is no Triage.
func (s *Store) LoadLive() (*Record, error) {
	raw, err := os.ReadFile(s.markerPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	rec, err := s.Load(strings.TrimSpace(string(raw)))
	if err != nil || rec == nil || rec.State != StateLive {
		return nil, err
	}
	return rec, nil
}

// History lists the records that are not live, newest first.
func (s *Store) History() ([]*Record, error) {
	entries, err := os.ReadDir(s.dir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Record
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		rec, err := s.Load(id)
		if err != nil {
			return nil, err
		}
		if rec != nil && rec.State != StateLive {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.After(out[j].StartedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}
