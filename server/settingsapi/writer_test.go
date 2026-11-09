package settingsapi

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/config/registry"
)

func newWriter(path string) *Writer {
	return &Writer{Registry: registry.Default(), GlobalPath: path}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func noTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestWriterKeepsKeysAndOrderAndAppendsNewKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `{"disk-space":{"warn-below":"1G"},"agent-runner":"pty"}`)

	res, err := newWriter(path).Write(Target{}, []Change{set(`"5m"`, "execution-queue", "spin-window")})
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Values["execution-queue.spin-window"]) != `"5m"` {
		t.Errorf("values = %v", res.Values)
	}
	want := "{\n  \"disk-space\": {\n    \"warn-below\": \"1G\"\n  },\n  \"agent-runner\": \"pty\",\n  \"execution-queue\": {\n    \"spin-window\": \"5m\"\n  }\n}\n"
	if got := readFile(t, path); got != want {
		t.Fatalf("got %q", got)
	}
}

func TestWriterHandEditBetweenLoadAndSaveSurvives(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `{"agent-runner":"pty"}`)
	w := newWriter(path)
	writeFile(t, path, `{"agent-runner":"pty","disk-space":{"warn-below":"2G"}}`)

	if _, err := w.Write(Target{}, []Change{set(`"5m"`, "execution-queue", "spin-window")}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, `"warn-below": "2G"`) || !strings.Contains(got, `"spin-window": "5m"`) {
		t.Fatalf("got %q", got)
	}
}

func TestWriterConcurrentWritesBothSurvive(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, path, `{}`)
	w := newWriter(path)
	var wg sync.WaitGroup
	for _, c := range []Change{
		set(`"5m"`, "execution-queue", "spin-window"),
		set(`"1G"`, "disk-space", "warn-below"),
	} {
		wg.Go(func() {
			if _, err := w.Write(Target{}, []Change{c}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	got := readFile(t, path)
	if !strings.Contains(got, `"spin-window"`) || !strings.Contains(got, `"warn-below"`) {
		t.Fatalf("got %q", got)
	}
}

func TestWriterSymlinkKeepsLinkAndUpdatesTarget(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "real.json")
	link := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, target, `{"agent-runner":"pty"}`)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := newWriter(link).Write(Target{}, []Change{set(`"5m"`, "execution-queue", "spin-window")}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link lost: %v %v", fi, err)
	}
	got := readFile(t, target)
	if !strings.Contains(got, `"agent-runner"`) || !strings.Contains(got, `"spin-window": "5m"`) {
		t.Fatalf("target = %q", got)
	}
	noTempFiles(t, filepath.Dir(target))
}

func TestWriterMissingFileCreatedForSetLeftForRemove(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	w := newWriter(path)
	if _, err := w.Write(Target{}, []Change{remove("execution-queue", "spin-window")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove created the file: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove created the directory: %v", err)
	}
	if _, err := w.Write(Target{}, []Change{set(`"5m"`, "execution-queue", "spin-window")}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, path), `"spin-window": "5m"`) {
		t.Fatal("file not created with the key")
	}
}

func TestWriterRefusalLeavesFileUntouched(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	const orig = `{"agent-runner":"pty"}`
	writeFile(t, path, orig)

	_, err := newWriter(path).Write(Target{}, []Change{set(`"not a duration"`, "execution-queue", "spin-window")})
	if _, ok := errors.AsType[*Refusal](err); !ok {
		t.Fatalf("err = %v", err)
	}
	if got := readFile(t, path); got != orig {
		t.Fatalf("file changed: %q", got)
	}
	noTempFiles(t, dir)
}

type fakeHooks struct {
	refuse error
	notice string
	before int
	after  int
}

func (h *fakeHooks) Before(Target, []Change) error { h.before++; return h.refuse }
func (h *fakeHooks) After(Target, []Change) string { h.after++; return h.notice }

func TestWriterHookRefusalWritesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	const orig = `{"agent-runner":"pty"}`
	writeFile(t, path, orig)
	hooks := &fakeHooks{refuse: refuse("nope")}
	w := newWriter(path)
	w.Hooks = hooks

	_, err := w.Write(Target{}, []Change{set(`"5m"`, "execution-queue", "spin-window")})
	if _, ok := errors.AsType[*Refusal](err); !ok {
		t.Fatalf("err = %v", err)
	}
	if got := readFile(t, path); got != orig || hooks.after != 0 {
		t.Fatalf("file = %q, after calls = %d", got, hooks.after)
	}
	noTempFiles(t, dir)
}

func TestWriterAfterHookNoticeInResult(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	w := newWriter(path)
	w.Hooks = &fakeHooks{notice: "heads up"}
	res, err := w.Write(Target{}, []Change{set(`"5m"`, "execution-queue", "spin-window")})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notices) != 1 || res.Notices[0] != "heads up" {
		t.Fatalf("notices = %v", res.Notices)
	}
}

func TestWriterProjectTargetResolvesThroughRegistry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := newWriter(filepath.Join(t.TempDir(), "config.json"))
	w.ProjectDir = func(p string) (string, error) {
		if p != "demo" {
			return "", errors.New("no project " + p)
		}
		return dir, nil
	}
	writeFile(t, filepath.Join(dir, config.ProjectFileName), `{}`)

	if _, err := w.Write(Target{Project: "demo"}, []Change{set(`"develop"`, "trunk")}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, filepath.Join(dir, config.ProjectFileName)), "develop") {
		t.Fatal("project.json not updated")
	}
	if _, err := w.Write(Target{Project: "other"}, nil); err == nil {
		t.Fatal("unknown project accepted")
	}
	if _, err := os.Stat(w.GlobalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("global file touched: %v", err)
	}
}
