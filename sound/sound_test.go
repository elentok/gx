package sound

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/config"
)

type fakeRunner struct {
	calls [][]string
	fail  map[string]bool // paths whose playback fails
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	if len(args) > 0 && f.fail[args[0]] {
		return errors.New("cannot play")
	}
	return nil
}

type fixture struct {
	runner     *fakeRunner
	configDir  string
	builtinDir string
	logs       *bytes.Buffer
}

func newFixture(t *testing.T, goos string) (*fixture, Player) {
	t.Helper()
	f := &fixture{
		runner:     &fakeRunner{fail: map[string]bool{}},
		configDir:  t.TempDir(),
		builtinDir: filepath.Join(t.TempDir(), "sounds"),
		logs:       &bytes.Buffer{},
	}
	log := slog.New(slog.NewTextHandler(f.logs, nil))
	return f, New(goos, f.runner.run, f.configDir, f.builtinDir, log)
}

var enabled = config.AgentsSoundsConfig{Enabled: true}

func TestPlay_DarwinPlaysBuiltinWithAfplay(t *testing.T) {
	for _, name := range []Name{Attention, Done} {
		t.Run(string(name), func(t *testing.T) {
			f, p := newFixture(t, "darwin")
			p.Play(context.Background(), enabled, name)

			want := filepath.Join(f.builtinDir, string(name)+".m4a")
			if len(f.runner.calls) != 1 || strings.Join(f.runner.calls[0], " ") != "afplay "+want {
				t.Fatalf("calls = %v, want [afplay %s]", f.runner.calls, want)
			}
			got, err := os.ReadFile(want)
			if err != nil {
				t.Fatal(err)
			}
			embedded, _ := builtins.ReadFile("sounds/" + string(name) + ".m4a")
			if !bytes.Equal(got, embedded) {
				t.Fatal("built-in file on disk differs from the embedded sound")
			}
		})
	}
}

func TestPlay_OtherOSRunsNothing(t *testing.T) {
	f, p := newFixture(t, "linux")
	p.Play(context.Background(), enabled, Done)
	if len(f.runner.calls) != 0 {
		t.Fatalf("calls = %v, want none", f.runner.calls)
	}
}

func TestPlay_DisabledPlaysNothing(t *testing.T) {
	f, p := newFixture(t, "darwin")
	p.Play(context.Background(), config.AgentsSoundsConfig{Enabled: false}, Done)
	if len(f.runner.calls) != 0 {
		t.Fatalf("calls = %v, want none", f.runner.calls)
	}
}

func TestPlay_BuiltinWrittenOnce(t *testing.T) {
	f, p := newFixture(t, "darwin")
	p.Play(context.Background(), enabled, Done)
	path := filepath.Join(f.builtinDir, "done.m4a")
	if err := os.WriteFile(path, []byte("marker"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.Play(context.Background(), enabled, Done)
	got, _ := os.ReadFile(path)
	if string(got) != "marker" {
		t.Fatal("built-in sound was written again on the second play")
	}
}

func TestPlay_CustomPathRelativeToConfigDir(t *testing.T) {
	f, p := newFixture(t, "darwin")
	custom := filepath.Join(f.configDir, "my-done.wav")
	if err := os.WriteFile(custom, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p.Play(context.Background(), config.AgentsSoundsConfig{Enabled: true, Done: "my-done.wav"}, Done)
	if len(f.runner.calls) != 1 || f.runner.calls[0][1] != custom {
		t.Fatalf("calls = %v, want afplay %s", f.runner.calls, custom)
	}
}

func TestPlay_MissingCustomFallsBackWithWarning(t *testing.T) {
	f, p := newFixture(t, "darwin")
	p.Play(context.Background(), config.AgentsSoundsConfig{Enabled: true, Attention: "nope.wav"}, Attention)

	want := filepath.Join(f.builtinDir, "attention.m4a")
	if len(f.runner.calls) != 1 || f.runner.calls[0][1] != want {
		t.Fatalf("calls = %v, want afplay %s", f.runner.calls, want)
	}
	if !strings.Contains(f.logs.String(), "level=WARN") || !strings.Contains(f.logs.String(), "nope.wav") {
		t.Fatalf("no warning naming the custom file in logs: %q", f.logs.String())
	}
}

func TestPlay_FailingCustomFallsBackWithWarning(t *testing.T) {
	f, p := newFixture(t, "darwin")
	custom := filepath.Join(f.configDir, "broken.wav")
	if err := os.WriteFile(custom, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.runner.fail[custom] = true
	p.Play(context.Background(), config.AgentsSoundsConfig{Enabled: true, Done: "broken.wav"}, Done)

	want := filepath.Join(f.builtinDir, "done.m4a")
	if len(f.runner.calls) != 2 || f.runner.calls[1][1] != want {
		t.Fatalf("calls = %v, want custom then built-in %s", f.runner.calls, want)
	}
	if !strings.Contains(f.logs.String(), "level=WARN") {
		t.Fatalf("no warning in logs: %q", f.logs.String())
	}
}

func TestNewSystemPlayer_NoopInTests(t *testing.T) {
	if _, ok := NewSystemPlayer(slog.Default()).(noop); !ok {
		t.Fatal("NewSystemPlayer inside a test binary should be a no-op")
	}
}
