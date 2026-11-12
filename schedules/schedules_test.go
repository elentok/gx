package schedules

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "proj", "project.json"), []byte(`{"name":"proj"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewStore(dir), dir
}

func valid() Schedule {
	return Schedule{
		Name:   "Nightly Report",
		Prompt: "Summarise the day.\n\nBe brief.",
		Days:   []time.Weekday{time.Monday, time.Friday},
		Times:  []string{"09:30", "08:00"},
	}
}

func TestRoundTrip(t *testing.T) {
	st, dir := newStore(t)
	created, err := st.Create("proj", valid())
	if err != nil {
		t.Fatal(err)
	}
	if created.Slug != "nightly-report" || created.LateLimitHours != 2 {
		t.Fatalf("created = %+v", created)
	}
	if _, err := os.Stat(filepath.Join(dir, "proj", ".schedules", "nightly-report.md")); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get("proj", "nightly-report")
	if err != nil {
		t.Fatal(err)
	}
	if got.Prompt != valid().Prompt || got.Project != "proj" || !reflect.DeepEqual(got.Times, []string{"08:00", "09:30"}) {
		t.Fatalf("got = %+v", got)
	}
	items, err := st.List()
	if err != nil || len(items) != 1 || !items[0].Registered || items[0].Slug != "nightly-report" {
		t.Fatalf("List = %+v, %v", items, err)
	}

	upd := valid()
	upd.Name = "Renamed"
	upd.LateLimitHours = 0.5
	updated, err := st.Update("proj", "nightly-report", upd)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Slug != "nightly-report" || updated.Name != "Renamed" || updated.LateLimitHours != 0.5 {
		t.Fatalf("updated = %+v", updated)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) || updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Fatalf("times: created %v, updated %+v", created.CreatedAt, updated)
	}
	if again, _ := st.List(); len(again) != 1 {
		t.Fatalf("rename made %d files", len(again))
	}

	if err := st.Delete("proj", "nightly-report"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get("proj", "nightly-report"); err != ErrNotFound {
		t.Fatalf("Get after delete = %v", err)
	}
}

func TestCreate_SlugCollisionRefused(t *testing.T) {
	st, _ := newStore(t)
	if _, err := st.Create("proj", valid()); err != nil {
		t.Fatal(err)
	}
	other := valid()
	other.Name = "nightly   report!"
	if _, err := st.Create("proj", other); ReasonOf(err) != ReasonSlugExists {
		t.Fatalf("err = %v, want %s", err, ReasonSlugExists)
	}
}

func TestUpdate_ProjectChangeRefused(t *testing.T) {
	st, _ := newStore(t)
	if _, err := st.Create("proj", valid()); err != nil {
		t.Fatal(err)
	}
	s := valid()
	s.Project = "other"
	if _, err := st.Update("proj", "nightly-report", s); ReasonOf(err) != ReasonInvalidProject {
		t.Fatalf("err = %v, want %s", err, ReasonInvalidProject)
	}
}

func TestCreate_UnknownProjectRefused(t *testing.T) {
	st, _ := newStore(t)
	for _, p := range []string{"missing", "", "../x", ".hidden"} {
		if _, err := st.Create(p, valid()); ReasonOf(err) != ReasonInvalidProject {
			t.Errorf("project %q: err = %v", p, err)
		}
	}
}

func TestValidate_Reasons(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Schedule)
		want   string
	}{
		{"empty name", func(s *Schedule) { s.Name = "  " }, ReasonEmptyName},
		{"empty prompt", func(s *Schedule) { s.Prompt = "" }, ReasonEmptyPrompt},
		{"no days", func(s *Schedule) { s.Days = nil }, ReasonNoDays},
		{"no times", func(s *Schedule) { s.Times = nil }, ReasonNoTimes},
		{"bad time 24:00", func(s *Schedule) { s.Times = []string{"24:00"} }, ReasonInvalidTime},
		{"bad time 9:30", func(s *Schedule) { s.Times = []string{"9:30"} }, ReasonInvalidTime},
		{"bad time minutes", func(s *Schedule) { s.Times = []string{"10:60"} }, ReasonInvalidTime},
		{"negative late limit", func(s *Schedule) { s.LateLimitHours = -1 }, ReasonInvalidLateLimit},
		{"bad harness", func(s *Schedule) { s.Harness = "gemini" }, ReasonInvalidHarness},
		{"ok codex", func(s *Schedule) { s.Harness = "codex" }, ""},
		{"ok decimal late limit", func(s *Schedule) { s.LateLimitHours = 1.5 }, ""},
		{"ok 23:59", func(s *Schedule) { s.Times = []string{"00:00", "23:59"} }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := valid()
			tc.mutate(&s)
			if got := ReasonOf(Validate(&s)); got != tc.want {
				t.Fatalf("reason = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidate_DuplicateTimesCollapse(t *testing.T) {
	s := valid()
	s.Times = []string{"09:00", "09:00", "08:00"}
	s.Days = []time.Weekday{time.Friday, time.Friday}
	if err := Validate(&s); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Times, []string{"08:00", "09:00"}) || len(s.Days) != 1 {
		t.Fatalf("times = %v, days = %v", s.Times, s.Days)
	}
}

func TestLoad_UnknownFutureKeyStillLoads(t *testing.T) {
	st, dir := newStore(t)
	path := filepath.Join(dir, "proj", ".schedules", "cron.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nversion: 1\nname: Cron\ncron: \"0 9 * * 1\"\ndays: [1]\ntimes: [\"09:00\"]\nlate_limit_hours: 2\n---\nDo it.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get("proj", "cron")
	if err != nil || got.Name != "Cron" || got.Prompt != "Do it." {
		t.Fatalf("got = %+v, %v", got, err)
	}
}

func TestList_UnregisteredProjectDirectory(t *testing.T) {
	st, dir := newStore(t)
	path := filepath.Join(dir, "gone", ".schedules", "a.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nversion: 1\nname: A\ndays: [2]\ntimes: [\"10:00\"]\nlate_limit_hours: 2\n---\nHi\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	items, err := st.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("items = %+v, %v", items, err)
	}
	if items[0].Registered || items[0].Project != "gone" || items[0].Slug != "a" {
		t.Fatalf("item = %+v", items[0])
	}
}
