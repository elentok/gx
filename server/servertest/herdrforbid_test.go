package servertest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/runnerfake"
)

func TestHerdr_NilWithoutWithHerdr(t *testing.T) {
	h := Start(t, WithRunner(runnerfake.NewRunner()))
	if h.Herdr != nil {
		t.Errorf("h.Herdr = %v, want nil for a WithRunner test", h.Herdr)
	}
}

func TestHerdr_OnTurnOnAWithRunnerHarnessFailsNamingOnTurn(t *testing.T) {
	h := Start(t, WithRunner(runnerfake.NewRunner()))
	rec := &failRecorder{TB: t}
	h.tb = rec

	h.OnTurn(func(*Turn) {})

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.errors) != 1 || !strings.Contains(rec.errors[0], "OnTurn") {
		t.Errorf("errors = %q, want one naming OnTurn", rec.errors)
	}
}

func TestHerdr_NilByDefault(t *testing.T) {
	if h := Start(t); h.Herdr != nil {
		t.Errorf("h.Herdr = %v, want nil on the default runner", h.Herdr)
	}
}

func TestHerdr_SetWithHerdr(t *testing.T) {
	if h := Start(t, WithRunner(runnerfake.NewRunner()), WithHerdr(HerdrIsTheSubject)); h.Herdr == nil {
		t.Error("h.Herdr = nil with WithHerdr, want the fake")
	}
}

func TestHerdr_WithHerdrKeepsTheConfiguredRunner(t *testing.T) {
	fake := runnerfake.NewRunner()
	h := Start(t, WithHerdr(HerdrIsTheSubject), WithRunner(fake))
	if h.cfg.Runner != fake {
		t.Errorf("configured runner = %v, want the fake runner", h.cfg.Runner)
	}
}

func TestHerdr_PathStartsWithTheFakeHerdrDir(t *testing.T) {
	h := Start(t, WithRunner(runnerfake.NewRunner()))
	first := strings.SplitN(h.Env("PATH"), ":", 2)[0]
	if _, err := os.Stat(filepath.Join(first, "herdr")); err != nil {
		t.Errorf("first PATH entry %q has no herdr executable: %v", first, err)
	}
}
