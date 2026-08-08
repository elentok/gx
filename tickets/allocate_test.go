package tickets

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNextTicketID_FlatSibling(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "01-first.md"),
		"---\nid: \"01\"\nstatus: done\ntype: task\n---\n")
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "02-second.md"),
		"---\nid: \"02\"\nstatus: done\ntype: task\n---\n")
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "03-third.md"),
		"---\nid: \"03\"\nstatus: done\ntype: task\n---\n")

	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	id, err := NextTicketID(epics[0], "")
	if err != nil {
		t.Fatalf("NextTicketID: %v", err)
	}
	if id != "04" {
		t.Errorf("NextTicketID(no parent) = %q, want %q", id, "04")
	}
}

func TestNextTicketID_EmptyEpicStartsAt01(t *testing.T) {
	id, err := NextTicketID(Epic{}, "")
	if err != nil {
		t.Fatalf("NextTicketID: %v", err)
	}
	if id != "01" {
		t.Errorf("NextTicketID(empty epic) = %q, want %q", id, "01")
	}
}

func TestNextTicketID_LetteredChildOfBareParent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "12-parent.md"),
		"---\nid: \"12\"\nstatus: done\ntype: task\n---\n")
	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	id, err := NextTicketID(epics[0], "12")
	if err != nil {
		t.Fatalf("NextTicketID: %v", err)
	}
	if id != "12a" {
		t.Errorf("NextTicketID(parent=12) = %q, want %q", id, "12a")
	}
}

func TestNextTicketID_LetteredChildSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "12-parent.md"),
		"---\nid: \"12\"\nstatus: done\ntype: task\n---\n")
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "12a-child.md"),
		"---\nid: \"12a\"\nstatus: done\ntype: task\n---\n")
	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	id, err := NextTicketID(epics[0], "12")
	if err != nil {
		t.Fatalf("NextTicketID: %v", err)
	}
	if id != "12b" {
		t.Errorf("NextTicketID(parent=12, 12a exists) = %q, want %q", id, "12b")
	}
}

func TestNextTicketID_NumericLevelPastLetteredParent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "12b-child.md"),
		"---\nid: \"12b\"\nstatus: done\ntype: task\n---\n")
	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	id, err := NextTicketID(epics[0], "12b")
	if err != nil {
		t.Fatalf("NextTicketID: %v", err)
	}
	if id != "12b1" {
		t.Errorf("NextTicketID(parent=12b) = %q, want %q", id, "12b1")
	}

	writeFile(t, filepath.Join(dir, "my-epic", "issues", "12b1-grandchild.md"),
		"---\nid: \"12b1\"\nstatus: done\ntype: task\n---\n")
	epics, err = Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	id, err = NextTicketID(epics[0], "12b")
	if err != nil {
		t.Fatalf("NextTicketID: %v", err)
	}
	if id != "12b2" {
		t.Errorf("NextTicketID(parent=12b, 12b1 exists) = %q, want %q", id, "12b2")
	}
}

func TestNextTicketID_LetteredNumericParentAllocatesNextSibling(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "02e-parent.md"),
		"---\nid: \"02e\"\nstatus: done\ntype: task\n---\n")
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "02e1-child.md"),
		"---\nid: \"02e1\"\nstatus: done\ntype: task\n---\n")
	writeFile(t, filepath.Join(dir, "my-epic", "issues", "02e2-child.md"),
		"---\nid: \"02e2\"\nstatus: done\ntype: task\n---\n")
	epics, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	id, err := NextTicketID(epics[0], "02e2")
	if err != nil {
		t.Fatalf("NextTicketID(parent=02e2): %v", err)
	}
	if id != "02e3" {
		t.Errorf("NextTicketID(parent=02e2) = %q, want %q", id, "02e3")
	}

	id, err = NextTicketID(epics[0], "02e")
	if err != nil {
		t.Fatalf("NextTicketID(parent=02e): %v", err)
	}
	if id != "02e3" {
		t.Errorf("NextTicketID(parent=02e) = %q, want %q", id, "02e3")
	}
}

func TestLoadLockedEpic_ValidPath(t *testing.T) {
	dir := t.TempDir()
	epicPath := filepath.Join(dir, "my-epic")
	writeFile(t, filepath.Join(epicPath, "issues", "01-first.md"),
		"---\nid: \"01\"\nstatus: done\ntype: task\n---\n")

	epic, unlock, err := LoadLockedEpic(epicPath)
	if err != nil {
		t.Fatalf("LoadLockedEpic: %v", err)
	}
	defer unlock()

	if epic.Name != "my-epic" {
		t.Errorf("epic.Name = %q, want %q", epic.Name, "my-epic")
	}
	if _, err := os.Stat(filepath.Join(epicPath, allocLockFileName)); err != nil {
		t.Errorf("expected lock file to exist while held: %v", err)
	}
}

func TestLoadLockedEpic_MissingPathReturnsErrorAndDoesNotLeaveLock(t *testing.T) {
	dir := t.TempDir()
	epicPath := filepath.Join(dir, "does-not-exist")

	epic, unlock, err := LoadLockedEpic(epicPath)
	if err == nil {
		t.Fatal("expected error for missing epic path, got nil")
	}
	if epic != nil {
		t.Errorf("expected nil epic on error, got %+v", epic)
	}
	if unlock != nil {
		t.Error("expected nil unlock on error")
	}
	if _, statErr := os.Stat(filepath.Join(epicPath, allocLockFileName)); !os.IsNotExist(statErr) {
		t.Errorf("expected no lock file left behind, stat err = %v", statErr)
	}
}

func TestLoadLockedEpic_EmptyEpic(t *testing.T) {
	dir := t.TempDir()
	epicPath := filepath.Join(dir, "my-epic")
	if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	epic, unlock, err := LoadLockedEpic(epicPath)
	if err != nil {
		t.Fatalf("LoadLockedEpic: %v", err)
	}
	defer unlock()
	if epic.Name != "my-epic" {
		t.Errorf("epic.Name = %q, want %q", epic.Name, "my-epic")
	}
}

func TestLockEpic_SerializesConcurrentAllocation(t *testing.T) {
	dir := t.TempDir()
	epicPath := filepath.Join(dir, "my-epic")
	if err := os.MkdirAll(filepath.Join(epicPath, "issues"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	const n = 20
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			unlock, err := LockEpic(epicPath)
			if err != nil {
				errs[i] = err
				return
			}
			defer unlock()

			epics, err := Load(dir)
			if err != nil {
				errs[i] = err
				return
			}
			var epic Epic
			for _, e := range epics {
				if e.Name == "my-epic" {
					epic = e
					break
				}
			}
			id, err := NextTicketID(epic, "")
			if err != nil {
				errs[i] = err
				return
			}
			ids[i] = id
			path := filepath.Join(epicPath, "issues", id+"-stub.md")
			content := "---\nid: \"" + id + "\"\nstatus: open\ntype: task\n---\n"
			errs[i] = os.WriteFile(path, []byte(content), 0644)
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		if seen[ids[i]] {
			t.Fatalf("duplicate allocated ID %q", ids[i])
		}
		seen[ids[i]] = true
	}
	if len(seen) != n {
		t.Fatalf("expected %d unique IDs, got %d", n, len(seen))
	}
}
