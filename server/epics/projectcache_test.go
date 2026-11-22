package epics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

// cachedStore is a one-project store whose project.json reads are counted and
// whose generation the test moves by hand.
type cachedStore struct {
	dir   string
	reads int
	gen   uint64
}

func newCachedStore(t *testing.T) (*cachedStore, Deps) {
	t.Helper()
	cs := &cachedStore{dir: t.TempDir()}
	cs.writeProject(t, "alpha", "alpha")
	cache := tickets.NewProjectCache(func() uint64 { return cs.gen })
	cache.Read = func(dir string) (config.ProjectFile, error) {
		cs.reads++
		return config.ReadProjectFile(dir)
	}
	d := fakeEpics()
	d.TicketStore = cs.dir
	d.ProjectCache = cache
	return cs, d
}

func (cs *cachedStore) writeProject(t *testing.T, dirName, name string) {
	t.Helper()
	dir := filepath.Join(cs.dir, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{"name":"`+name+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func epicListOf(t *testing.T, s *Epics) wire.EpicList {
	t.Helper()
	rec := httptest.NewRecorder()
	s.EpicList(rec, httptest.NewRequest(http.MethodGet, "/v1/epics", nil))
	var list wire.EpicList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func TestEpicList_ProjectFilesAreReadOncePerStoreGeneration(t *testing.T) {
	cs, d := newCachedStore(t)
	s := New(d)

	epicListOf(t, s)
	s.NeedsYouCounts()
	epicListOf(t, s)
	if cs.reads != 1 {
		t.Fatalf("project.json reads = %d across three card builds with no store change, want 1", cs.reads)
	}

	cs.writeProject(t, "alpha", "renamed")
	cs.writeProject(t, "beta", "beta")
	cs.gen++
	list := epicListOf(t, s)
	if !slices.Equal(list.Projects, []string{"renamed", "beta"}) {
		t.Fatalf("projects after a rescan = %v, want [renamed beta]", list.Projects)
	}
}

func TestEpicList_LiveInputsStayFreshWithNoStoreChange(t *testing.T) {
	cs, d := newCachedStore(t)
	d.Projects = &fakeProjects{epics: map[string][]tickets.Epic{"alpha": {{
		Name:    "e",
		Tickets: []tickets.Ticket{{Number: 1, Identifier: "1", Status: "open"}},
	}}}}
	s := New(d)

	slot := func() int {
		for _, r := range epicListOf(t, s).Epics {
			if r.Address == "alpha:e" {
				return r.QueueSlot
			}
		}
		t.Fatal("no card for alpha:e")
		return 0
	}
	if got := slot(); got != 0 {
		t.Fatalf("slot before queueing = %d, want 0", got)
	}
	s.Queue = fakeQueue{items: []wire.QueueItem{{Address: "alpha:e/01"}}}
	if got := slot(); got != 1 {
		t.Fatalf("slot after queueing = %d, want 1 with no store change (reads = %d)", got, cs.reads)
	}
}
