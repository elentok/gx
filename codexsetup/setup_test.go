package codexsetup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func run(t *testing.T, o Options) (string, error) {
	t.Helper()
	var out bytes.Buffer
	o.Out = &out
	if o.In == nil {
		o.In = strings.NewReader("")
	}
	err := Run(o)
	return out.String(), err
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRun_FreshDirWritesRulesFile(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	out, err := run(t, Options{CodexHome: home, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	p := RulesPath(home)
	if !strings.HasPrefix(read(t, p), Marker+"\n") {
		t.Errorf("rules file lacks marker")
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
	if !strings.Contains(out, "Restart Codex for the changes to take effect.") {
		t.Errorf("missing restart line: %q", out)
	}
}

func TestRun_SecondRunChangesNothing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if _, err := run(t, Options{CodexHome: home, Yes: true}); err != nil {
		t.Fatal(err)
	}
	rules := RulesPath(home)
	before := read(t, rules)
	fi, _ := os.Stat(rules)
	time.Sleep(20 * time.Millisecond)

	out, err := run(t, Options{CodexHome: home, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "already set up") {
		t.Errorf("out = %q, want already set up", out)
	}
	if read(t, rules) != before {
		t.Errorf("bytes changed on second run")
	}
	fi2, _ := os.Stat(rules)
	if !fi.ModTime().Equal(fi2.ModTime()) {
		t.Errorf("mtime changed on second run")
	}
}

func TestRun_UnmanagedRulesFileUntouched(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	write(t, RulesPath(home), "# mine\n")
	out, err := run(t, Options{CodexHome: home, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, RulesPath(home)) != "# mine\n" {
		t.Errorf("unmanaged file was changed")
	}
	if !strings.Contains(out, "not managed by gx") {
		t.Errorf("out = %q, want unmanaged note", out)
	}
}

func TestRun_OldMarkerUpdatedInPlace(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	write(t, RulesPath(home), OldMarker+"\nold body\n")
	if _, err := run(t, Options{CodexHome: home, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, RulesPath(home)); got != RulesContent() {
		t.Errorf("rules not updated:\n%s", got)
	}
}

func TestRun_PreviewNamesPathsBeforeWrite(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	out, err := run(t, Options{CodexHome: home, Interactive: true, In: strings.NewReader("n\n")})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{RulesPath(home), "Write these changes? [y/N]"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(RulesPath(home)); err == nil {
		t.Errorf("rules file written after answering n")
	}
}

func TestRun_YesAnswerWrites(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if _, err := run(t, Options{CodexHome: home, Interactive: true, In: strings.NewReader("y\n")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(RulesPath(home)); err != nil {
		t.Errorf("rules file not written: %v", err)
	}
}

func TestRun_NoTTYWithoutYesRefuses(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if _, err := run(t, Options{CodexHome: home}); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(RulesPath(home)); err == nil {
		t.Errorf("rules file written without tty or --yes")
	}
}
