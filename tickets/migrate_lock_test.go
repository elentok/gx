package tickets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/tickets/schema"
)

const legacyTicket = "---\nid: \"01\"\nstatus: ready-for-agent\ntype: implement\n---\nBody.\n"

func migrateWithHook(t *testing.T, hook func(path string)) (path string, err error) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, "my-epic", "issues", "01-first.md")
	writeFile(t, path, legacyTicket)
	beforeMigrationWrite = func() { hook(path) }
	t.Cleanup(func() { beforeMigrationWrite = nil })
	_, err = Migrate(dir)
	return path, err
}

func TestMigrate_KeepsWriteLandedBetweenCollectAndWrite(t *testing.T) {
	path, err := migrateWithHook(t, func(path string) {
		unlock, err := schema.LockTicket(path)
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		writeFile(t, path, strings.Replace(legacyTicket, "Body.", "Body.\n\nAgent note.", 1))
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "Agent note.") {
		t.Errorf("concurrent write lost:\n%s", got)
	}
	if !strings.Contains(string(got), "status: open") {
		t.Errorf("migration not applied:\n%s", got)
	}
}

func TestMigrate_FreshContentsFailingValidationFailTheRun(t *testing.T) {
	invalid := "---\nid: \"01\"\nstatus: ready-for-agent\ntype: nonsense\n---\nBody.\n"
	path, err := migrateWithHook(t, func(path string) { writeFile(t, path, invalid) })
	if err == nil {
		t.Fatal("Migrate succeeded, want a validation error")
	}
	if got, _ := os.ReadFile(path); string(got) != invalid {
		t.Errorf("file changed:\n%s", got)
	}
}

func TestMigrate_HeldLockPastTimeoutFailsWithoutWriting(t *testing.T) {
	defer schema.SetLockTimeoutForTest(50 * time.Millisecond)()
	var release func()
	path, err := migrateWithHook(t, func(path string) {
		unlock, err := schema.LockTicket(path)
		if err != nil {
			t.Fatal(err)
		}
		release = unlock
	})
	defer release()
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a lock timeout", err)
	}
	if got, _ := os.ReadFile(path); string(got) != legacyTicket {
		t.Errorf("file changed:\n%s", got)
	}
}
