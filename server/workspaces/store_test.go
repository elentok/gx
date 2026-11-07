package workspaces

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"os"
	"path/filepath"

	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/wire"
)

// fakeDesigns answers only DesignExists; the embedded nil Directory panics on
// anything else, which these tests never reach.
type fakeDesigns struct {
	Directory
	designs map[string]bool
}

func (f fakeDesigns) DesignExists(key string) bool { return f.designs[key] }

func open(t *testing.T, dir string, b *broker.Broker) *Store {
	t.Helper()
	d := fakeDesigns{designs: map[string]bool{"p:nav": true}}
	s, err := Open(dir, b, func() Directory { return d })
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func names(s *Store) []string {
	var out []string
	for _, w := range s.Listed() {
		out = append(out, w.Name)
	}
	return out
}

func TestRefusals(t *testing.T) {
	s := open(t, t.TempDir(), broker.New(0))
	if err := s.Create("Bugs"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		do   func() error
		want Reason
	}{
		{"duplicate ignoring case", func() error { return s.Create("BUGS") }, ReasonNameTaken},
		{"blank", func() error { return s.Create("   ") }, ReasonBlankName},
		{"default name taken", func() error { return s.Create("default WORKSPACE") }, ReasonNameTaken},
		{"delete default", func() error { return s.Delete(DefaultName) }, ReasonDefaultFixed},
		{"rename default", func() error { return s.Rename(DefaultName, "Home") }, ReasonDefaultFixed},
		{"rename onto another", func() error { return s.Rename("Bugs", "default workspace") }, ReasonNameTaken},
		{"rename to blank", func() error { return s.Rename("Bugs", " ") }, ReasonBlankName},
		{"delete unknown", func() error { return s.Delete("Nope") }, ReasonUnknown},
		{"member of unknown", func() error { return s.SetMember("p:a", "Nope") }, ReasonUnknown},
		{"sort unknown value", func() error { return s.SetSort("Bugs", "random") }, ReasonBadSort},
		{"sort unknown workspace", func() error { return s.SetSort("Nope", wire.SortName) }, ReasonUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.do(); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
	if got := names(s); !slices.Equal(got, []string{DefaultName, "Bugs"}) {
		t.Fatalf("list = %v", got)
	}
}

func TestOrder(t *testing.T) {
	s := open(t, t.TempDir(), broker.New(0))
	for _, n := range []string{"A", "B"} {
		if err := s.Create(n); err != nil {
			t.Fatal(err)
		}
	}
	if got := names(s); !slices.Equal(got, []string{DefaultName, "A", "B"}) {
		t.Fatalf("new last: %v", got)
	}
	if err := s.Move(DefaultName, 2); err != nil {
		t.Fatal(err)
	}
	if got := names(s); !slices.Equal(got, []string{"A", "B", DefaultName}) {
		t.Fatalf("default moved: %v", got)
	}
	if err := s.Move("b", 0); err != nil {
		t.Fatal(err)
	}
	if got := names(s); !slices.Equal(got, []string{"B", "A", DefaultName}) {
		t.Fatalf("b first: %v", got)
	}
}

func TestMembership(t *testing.T) {
	s := open(t, t.TempDir(), broker.New(0))
	if got := s.Workspace("p:unknown"); got != DefaultName {
		t.Fatalf("unknown key = %q", got)
	}
	if err := s.Create("Bugs"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMember("p:a", "bugs"); err != nil {
		t.Fatal(err)
	}
	if got := s.Workspace("p:a"); got != "Bugs" {
		t.Fatalf("member = %q", got)
	}
	if err := s.Rename("Bugs", "Fixes"); err != nil {
		t.Fatal(err)
	}
	if got := s.Workspace("p:a"); got != "Fixes" {
		t.Fatalf("after rename = %q", got)
	}
	if err := s.Delete("Fixes"); err != nil {
		t.Fatal(err)
	}
	if got := s.Workspace("p:a"); got != DefaultName {
		t.Fatalf("after delete = %q", got)
	}
}

func TestDefaultMemberStoresNothing(t *testing.T) {
	s := open(t, t.TempDir(), broker.New(0))
	if err := s.Create("Bugs"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMember("p:a", "Bugs"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMember("p:a", DefaultName); err != nil {
		t.Fatal(err)
	}
	if len(s.st.Members) != 0 {
		t.Fatalf("members = %v, want none", s.st.Members)
	}
	if err := s.Remove("p:a"); err != nil {
		t.Fatalf("remove of a default member = %v, want nil", err)
	}
}

func TestImplFollowsDesign(t *testing.T) {
	s := open(t, t.TempDir(), broker.New(0))
	if err := s.Create("Nav"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMember("p:nav-impl", "Nav"); err != nil {
		t.Fatal(err)
	}
	if got := s.Workspace("p:nav"); got != "Nav" {
		t.Fatalf("design = %q", got)
	}
	if err := s.Hold("p:nav"); err != nil {
		t.Fatal(err)
	}
	if !s.IsHeld("p:nav-impl") {
		t.Fatal("impl should read its design's hold")
	}
	// Without a Design the -impl key stands alone.
	if err := s.SetMember("p:other-impl", "Nav"); err != nil {
		t.Fatal(err)
	}
	if got := s.Workspace("p:other"); got != DefaultName {
		t.Fatalf("no design: %q", got)
	}
}

func TestHold(t *testing.T) {
	s := open(t, t.TempDir(), broker.New(0))
	if err := s.Create("Bugs"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMember("p:a", "Bugs"); err != nil {
		t.Fatal(err)
	}
	if err := s.Hold("p:a"); err != nil {
		t.Fatal(err)
	}
	if !s.IsHeld("p:a") || s.Workspace("p:a") != "Bugs" {
		t.Fatal("held key should be held and keep its workspace")
	}
	if err := s.Unhold("p:a"); err != nil {
		t.Fatal(err)
	}
	if s.IsHeld("p:a") {
		t.Fatal("still held")
	}
}

func TestPrune(t *testing.T) {
	s := open(t, t.TempDir(), broker.New(0))
	if err := s.Create("Bugs"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"p:gone", "p:kept"} {
		if err := s.SetMember(k, "Bugs"); err != nil {
			t.Fatal(err)
		}
		if err := s.Hold(k); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Prune(func(k string) bool { return k == "p:kept" }); err != nil {
		t.Fatal(err)
	}
	if s.Workspace("p:gone") != DefaultName || s.IsHeld("p:gone") {
		t.Fatal("gone key survived")
	}
	if s.Workspace("p:kept") != "Bugs" || !s.IsHeld("p:kept") {
		t.Fatal("kept key lost")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir, broker.New(0))
	for _, n := range []string{"A", "B"} {
		if err := s.Create(n); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Move(DefaultName, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMember("p:x", "B"); err != nil {
		t.Fatal(err)
	}
	if err := s.Hold("p:x"); err != nil {
		t.Fatal(err)
	}

	r := open(t, dir, broker.New(0))
	if got := names(r); !slices.Equal(got, []string{"A", DefaultName, "B"}) {
		t.Fatalf("list = %v", got)
	}
	if r.Workspace("p:x") != "B" || !r.IsHeld("p:x") {
		t.Fatal("membership or hold lost on reopen")
	}
}

func listedSort(s *Store, name string) string {
	for _, w := range s.Listed() {
		if w.Name == name {
			return w.Sort
		}
	}
	return "missing"
}

func TestSort(t *testing.T) {
	dir := t.TempDir()
	b := broker.New(0)
	s := open(t, dir, b)
	for _, n := range []string{"A", "B"} {
		if err := s.Create(n); err != nil {
			t.Fatal(err)
		}
	}
	if s.Sort("A") != wire.SortNeedsYou || listedSort(s, "A") != wire.SortNeedsYou {
		t.Fatal("a new workspace should read the default")
	}

	seq := b.CurrentSeq()
	if err := s.SetSort("a", wire.SortActivity); err != nil {
		t.Fatal(err)
	}
	if b.CurrentSeq() != seq+1 {
		t.Fatalf("seq after set = %d, want %d", b.CurrentSeq(), seq+1)
	}
	if s.Sort("A") != wire.SortActivity || listedSort(s, "A") != wire.SortActivity {
		t.Fatal("set value not read")
	}
	if s.Sort("B") != wire.SortNeedsYou {
		t.Fatal("another workspace changed")
	}
	if err := s.SetSort("A", wire.SortActivity); err != nil || b.CurrentSeq() != seq+1 {
		t.Fatalf("no-op set err=%v seq=%d", err, b.CurrentSeq())
	}

	if err := s.SetSort("B", "bogus"); !errors.Is(err, ReasonBadSort) || s.Sort("B") != wire.SortNeedsYou || b.CurrentSeq() != seq+1 {
		t.Fatalf("refusal stored or published: %v", err)
	}

	r := open(t, dir, broker.New(0))
	if r.Sort("A") != wire.SortActivity {
		t.Fatal("sort lost on reopen")
	}

	if err := s.Rename("A", "C"); err != nil {
		t.Fatal(err)
	}
	if s.Sort("C") != wire.SortActivity || s.Sort("A") != wire.SortNeedsYou {
		t.Fatal("rename did not move the sort")
	}
	if err := s.SetSort("C", wire.SortNeedsYou); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"C"`) && strings.Contains(string(data), `"sorts"`) {
		t.Fatalf("default value left an entry: %s", data)
	}

	if err := s.SetSort("C", wire.SortName); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("C"); err != nil {
		t.Fatal(err)
	}
	if err := s.Create("C"); err != nil {
		t.Fatal(err)
	}
	if s.Sort("C") != wire.SortNeedsYou {
		t.Fatal("delete kept the sort")
	}
}

func TestSortFromFile(t *testing.T) {
	dir := t.TempDir()
	old := `{"order":["Default workspace","A"],"members":{},"held":null}`
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := open(t, dir, broker.New(0)); s.Sort("A") != wire.SortNeedsYou {
		t.Fatal("file without sorts should read the default")
	}
	bad := `{"order":["Default workspace","A"],"members":{},"sorts":{"A":"weird"}}`
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	s := open(t, dir, broker.New(0))
	if s.Sort("A") != wire.SortNeedsYou || listedSort(s, "A") != wire.SortNeedsYou {
		t.Fatal("unknown value should read the default")
	}
}

func TestWritesPublish(t *testing.T) {
	b := broker.New(0)
	s := open(t, t.TempDir(), b)
	if err := s.Create("Bugs"); err != nil {
		t.Fatal(err)
	}
	if got := b.CurrentSeq(); got != 1 {
		t.Fatalf("seq after create = %d, want 1", got)
	}
	if err := s.Create("bugs"); err == nil {
		t.Fatal("duplicate accepted")
	}
	if got := b.CurrentSeq(); got != 1 {
		t.Fatalf("seq after refusal = %d, want 1", got)
	}
	if err := s.Hold("p:a"); err != nil {
		t.Fatal(err)
	}
	if got := b.CurrentSeq(); got != 2 {
		t.Fatalf("seq after hold = %d, want 2", got)
	}
}
