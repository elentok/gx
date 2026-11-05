package serverctl

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/migrations"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestMigrationHooks_UnsetChangeNothing(t *testing.T) {
	reg, err := migrationHooks(envOf(nil))
	if err != nil || reg != nil {
		t.Fatalf("got %v, %v; want nil, nil", reg, err)
	}
}

func TestMigrationHooks_InjectAddsFailingAndNotingStepsAfterTheRealOnes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "inject.json")
	body := `[{"name":"t-fail","fail":"boom"},{"name":"t-skip","notes":["skipped x"]}]`
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := migrationHooks(envOf(map[string]string{migrateInjectEnv: file}))
	if err != nil {
		t.Fatal(err)
	}
	want := append(migrations.Default.Names(), "t-fail", "t-skip")
	if got := reg.Names(); !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
}

func TestMigrationHooks_MissingInjectFileAddsNothing(t *testing.T) {
	reg, err := migrationHooks(envOf(map[string]string{migrateInjectEnv: filepath.Join(t.TempDir(), "none")}))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := reg.Names(), migrations.Default.Names(); !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
}

func TestMigrationHooks_HoldStepWaitsUntilTheFileIsRemoved(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := migrationHooks(envOf(map[string]string{migrateHoldEnv: hold}))
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.Names()[0]; got != "test-hold-open" {
		t.Fatalf("first step = %q, want the hold step", got)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = holdStep(hold).Run(migrations.Env{})
	}()
	select {
	case <-done:
		t.Fatal("migration ended while the hold file existed")
	case <-time.After(200 * time.Millisecond):
	}
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("migration still held after the file was removed")
	}
}
