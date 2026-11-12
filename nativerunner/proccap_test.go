package nativerunner

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/proccap"
)

// The holder argv carries the cap computed at launch, with the session id
// still in place for the liveness check.
func TestPtyLauncher_SpawnPassesTheCapToTheHolder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out := filepath.Join(dir, "args")
	l := ptyLauncher{
		holder:  []string{"/bin/sh", "-c", `printf '%s\n' "$@" > "$0"`, out},
		claude:  "/bin/true",
		procCap: func() int { return 1234 },
	}
	cmd, err := l.spawn(agentrunner.StartOptions{Cwd: dir}, dir, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Fields(string(data))
	i := slices.Index(args, "--proc-cap")
	if i < 0 || args[i+1] != "1234" {
		t.Errorf("holder args %v; want --proc-cap 1234", args)
	}
	if j := slices.Index(args, "--session-id"); j < 0 || args[j+1] != "sess-1" {
		t.Errorf("holder args %v; want --session-id sess-1", args)
	}
}

func TestPty_ProcCapReadsTheLiveHeadroomAtEachLaunch(t *testing.T) {
	t.Parallel()
	headroom := 10
	p := &Pty{ProcCap: proccap.Source{Headroom: func() int { return headroom }, Count: func() (int, error) { return 100, nil }}}
	if got := p.procCap(); got != 110 {
		t.Fatalf("cap = %d; want 110", got)
	}
	headroom = 20
	if got := p.procCap(); got != 120 {
		t.Fatalf("cap after a settings change = %d; want 120", got)
	}
	if got := (&Pty{}).procCap(); got != 0 {
		t.Fatalf("cap without headroom = %d; want none", got)
	}
}
