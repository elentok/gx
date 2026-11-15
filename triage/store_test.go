package triage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/tickets/tickettest"
)

func writeFollowUp(t *testing.T, projectDir, file, status, title string) {
	t.Helper()
	tickettest.WriteFile(t, filepath.Join(projectDir, "follow-ups", "issues", file),
		"---\nid: \""+file[:2]+"\"\nstatus: "+status+"\ntype: implement\n---\n\n# "+title+"\n\n## What to build\n\nx\n")
}

func newProject(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "proj")
	writeFollowUp(t, dir, "01-a.md", "open", "Alpha")
	writeFollowUp(t, dir, "02-b.md", "draft", "Beta")
	writeFollowUp(t, dir, "03-c.md", "done", "Gamma")
	return NewStore(dir), dir
}

func TestCreate_WritesRecordAndSnapshot(t *testing.T) {
	s, _ := newProject(t)
	rec, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != StateLive || rec.Step != "verify" || rec.Revision != 1 || rec.ID == "" || rec.StartedAt.IsZero() {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if len(rec.Rows) != 2 {
		t.Fatalf("snapshot should hold only draft and open follow-ups, got %+v", rec.Rows)
	}
	titles := []string{rec.Rows[0].Title, rec.Rows[1].Title}
	if titles[0] != "Alpha" || titles[1] != "Beta" {
		t.Fatalf("titles = %v", titles)
	}
	live, err := s.LoadLive()
	if err != nil || live == nil || live.ID != rec.ID || len(live.Rows) != 2 {
		t.Fatalf("LoadLive = %+v, %v", live, err)
	}
	byID, err := s.Load(rec.ID)
	if err != nil || byID == nil || byID.ID != rec.ID {
		t.Fatalf("Load = %+v, %v", byID, err)
	}
}

func TestCreate_WritesVersion2(t *testing.T) {
	s, dir := newProject(t)
	rec, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".triage", rec.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"version": 2`) {
		t.Fatalf("record is not version 2: %s", raw)
	}
}

const v1Row = `{"address":"proj:follow-ups/01","title":"Alpha","verdict":"relevant","write":"pending",
  "questions":[{"id":"q1","text":"Where?","kind":"quick","options":["a","b"],"default":"a"},{"id":"d1","text":"Store?","kind":"design"}],
  "plan":{"rowId":"proj:follow-ups/01","action":"map","destination":"m"},
  "person":{"action":"ready","destination":"x","answers":{"q1":"b"}}}`

// writeV1 stores a version 1 record and, when live, its marker. It returns the
// record file's path.
func writeV1(t *testing.T, dir, state, step string) string {
	t.Helper()
	folder := filepath.Join(dir, ".triage")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, "v1.json")
	body := `{"version":1,"id":"v1","startedAt":"2026-01-01T00:00:00Z","project":"proj","state":"` + state +
		`","step":"` + step + `","revision":3,"round":2,"rows":[` + v1Row + `]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if state == "live" {
		if err := os.WriteFile(filepath.Join(folder, "live"), []byte("v1"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestLoad_V1LoadNeverWritesAndSaveWritesV2(t *testing.T) {
	s, dir := newProject(t)
	path := writeV1(t, dir, "live", "verify")
	before, _ := os.ReadFile(path)
	rec, err := s.LoadLive()
	if err != nil || rec == nil {
		t.Fatalf("LoadLive = %+v, %v", rec, err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("a load rewrote the v1 record")
	}
	if rec.Step != "verify" || rec.Version != 2 {
		t.Fatalf("step %s version %d", rec.Step, rec.Version)
	}
	qs := rec.Rows[0].Questions
	if qs[0].Kind != QuestionAsk || qs[1].Kind != QuestionDesign {
		t.Fatalf("quick should read as ask and design stay: %+v", qs)
	}
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); !strings.Contains(string(after), `"version": 2`) {
		t.Fatalf("Save did not write version 2: %s", after)
	}
}

func TestLoad_V1AtPlanOrApproveGoesBackToRoute(t *testing.T) {
	for _, step := range []string{"plan", "approve"} {
		t.Run(step, func(t *testing.T) {
			s, dir := newProject(t)
			writeV1(t, dir, "live", step)
			rec, err := s.LoadLive()
			if err != nil || rec == nil {
				t.Fatalf("LoadLive = %+v, %v", rec, err)
			}
			row := rec.Rows[0]
			if rec.Step != "route" || Next(rec) != BranchRoute || rec.LegacyWrite() {
				t.Fatalf("step %s next %s", rec.Step, Next(rec))
			}
			if row.Verdict != "relevant" || row.Plan != nil || row.Person.Action != "" || len(row.Person.Answers) != 0 {
				t.Fatalf("verdict kept, plan and edits dropped: %+v", row)
			}
			if len(rec.Rounds) != 1 || !rec.Rounds[0].Answered || len(rec.Rounds[0].Questions) != 1 {
				t.Fatalf("rounds = %+v", rec.Rounds)
			}
			if q := rec.Rounds[0].Questions[0]; q.Kind != QuestionAsk || q.ID != "q1" || rec.Rounds[0].Answers["q1"] != "b" {
				t.Fatalf("the given answer becomes an answered ask question: %+v %v", q, rec.Rounds[0].Answers)
			}
			if len(row.Questions) != 1 || row.Questions[0].Kind != QuestionDesign {
				t.Fatalf("the design question stays on the row: %+v", row.Questions)
			}
		})
	}
}

func TestLoad_V1DoneAndAbandonedStayReadOnlyHistory(t *testing.T) {
	for _, state := range []string{"done", "abandoned"} {
		t.Run(state, func(t *testing.T) {
			s, dir := newProject(t)
			writeV1(t, dir, state, "plan")
			h, err := s.History()
			if err != nil || len(h) != 1 {
				t.Fatalf("History = %+v, %v", h, err)
			}
			rec := h[0]
			if rec.Step != "plan" || rec.Rows[0].Plan == nil || rec.Rows[0].Plan.Action != "map" || len(rec.Rounds) != 0 {
				t.Fatalf("history record changed: step %s row %+v", rec.Step, rec.Rows[0])
			}
		})
	}
}

func TestLoad_V1AtWriteOnlyAbandons(t *testing.T) {
	s, dir := newProject(t)
	writeV1(t, dir, "live", "write")
	rec, err := s.LoadLive()
	if err != nil || rec == nil || rec.Step != "write" || !rec.LegacyWrite() {
		t.Fatalf("LoadLive = %+v, %v", rec, err)
	}
	row := rec.Rows[0].Address
	var phase *PhaseError
	if _, err := s.WriteRow(rec.ID, rec.Revision, row, WriteDone, nil, ""); !errors.As(err, &phase) {
		t.Fatalf("WriteRow err = %v, want a PhaseError", err)
	}
	if _, err := s.SetLastPass(rec.ID, rec.Revision, true); !errors.As(err, &phase) {
		t.Fatalf("SetLastPass err = %v, want a PhaseError", err)
	}
	got, err := s.Abandon(rec.ID, rec.Revision)
	if err != nil || got.State != StateAbandoned {
		t.Fatalf("Abandon = %+v, %v", got, err)
	}
}

func TestAbandon_RefusedAtWriteForAV2Record(t *testing.T) {
	s, rec := atWrite(t)
	var phase *PhaseError
	if _, err := s.Abandon(rec.ID, rec.Revision); !errors.As(err, &phase) {
		t.Fatalf("Abandon err = %v, want a PhaseError", err)
	}
}

func TestCreate_RefusesWhileLive(t *testing.T) {
	s, _ := newProject(t)
	first, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Create()
	var live *LiveError
	if !errors.As(err, &live) || live.ID != first.ID {
		t.Fatalf("err = %v, want LiveError for %s", err, first.ID)
	}
}

func TestCreate_ConcurrentEndsWithOne(t *testing.T) {
	s, _ := newProject(t)
	const n = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, refused := 0, 0
	for range n {
		wg.Go(func() {
			_, err := s.Create()
			var live *LiveError
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.As(err, &live):
				refused++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	wg.Wait()
	if ok != 1 || refused != n-1 {
		t.Fatalf("ok=%d refused=%d", ok, refused)
	}
	entries, _ := filepath.Glob(filepath.Join(s.dir(), "*.json"))
	if len(entries) != 1 {
		t.Fatalf("records on disk = %v", entries)
	}
}

func TestCreate_ReplacesStaleMarker(t *testing.T) {
	s, _ := newProject(t)
	if err := os.MkdirAll(s.dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.markerPath(), []byte("20200101T000000-dead"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := s.Create()
	if err != nil {
		t.Fatalf("stale marker should be replaced: %v", err)
	}
	live, _ := s.LoadLive()
	if live == nil || live.ID != rec.ID {
		t.Fatalf("live = %+v", live)
	}
}

func TestSave_StaleRevisionRefused(t *testing.T) {
	s, _ := newProject(t)
	rec, _ := s.Create()
	other, _ := s.Load(rec.ID)

	rec.Step = "plan"
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	if rec.Revision != 2 {
		t.Fatalf("revision = %d, want 2", rec.Revision)
	}

	other.Step = "write"
	if err := s.Save(other); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("err = %v, want ErrStaleRevision", err)
	}
	got, _ := s.Load(rec.ID)
	if got.Step != "plan" || got.Revision != 2 {
		t.Fatalf("record changed by refused save: %+v", got)
	}
}

func TestSave_LeavingLiveReleasesMarker(t *testing.T) {
	s, _ := newProject(t)
	rec, _ := s.Create()
	rec.State = StateDone
	if err := s.Save(rec); err != nil {
		t.Fatal(err)
	}
	if live, err := s.LoadLive(); err != nil || live != nil {
		t.Fatalf("LoadLive = %+v, %v", live, err)
	}
	if _, err := s.Create(); err != nil {
		t.Fatalf("create after done: %v", err)
	}
}

func TestMissingFolderIsNoTriage(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "nothing"))
	if live, err := s.LoadLive(); live != nil || err != nil {
		t.Fatalf("LoadLive = %+v, %v", live, err)
	}
	if h, err := s.History(); len(h) != 0 || err != nil {
		t.Fatalf("History = %+v, %v", h, err)
	}
	if r, err := s.Load("x"); r != nil || err != nil {
		t.Fatalf("Load = %+v, %v", r, err)
	}
}

func TestHistory_NewestFirst(t *testing.T) {
	s, _ := newProject(t)
	var ids []string
	for i := range 3 {
		rec, err := s.Create()
		if err != nil {
			t.Fatal(err)
		}
		rec.State = StateAbandoned
		rec.StartedAt = time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
		if err := s.Save(rec); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec.ID)
	}
	h, err := s.History()
	if err != nil || len(h) != 3 {
		t.Fatalf("History = %d, %v", len(h), err)
	}
	if h[0].ID != ids[2] || h[1].ID != ids[1] || h[2].ID != ids[0] {
		t.Fatalf("order = %s %s %s, want %v reversed", h[0].ID, h[1].ID, h[2].ID, ids)
	}
}
