package cmd

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/wire"
)

func TestDetachedServerCmd_RunsUnderTheGivenEnv(t *testing.T) {
	env := []string{"HOME=/h", "PATH=/p"}
	if got := detachedServerCmd("gx", env, nil).Env; !slices.Equal(got, env) {
		t.Errorf("Env = %v, want %v", got, env)
	}
	if got := detachedServerCmd("gx", nil, nil).Env; got == nil {
		t.Error("an empty env must stay empty, not inherit the process env (nil Env)")
	}
}

func TestStatusWarnings(t *testing.T) {
	remote := t.TempDir()
	if err := exec.Command("git", "init", "-q", remote).Run(); err != nil {
		t.Fatal(err)
	}
	noRemote := statusWarnings(apiclient.Negotiation{}, remote)
	if len(noRemote) != 1 || !strings.Contains(noRemote[0], "no push remote") {
		t.Fatalf("no-remote warnings = %q", noRemote)
	}
	if err := exec.Command("git", "-C", remote, "remote", "add", "origin", "x:y").Run(); err != nil {
		t.Fatal(err)
	}
	if got := statusWarnings(apiclient.Negotiation{}, remote); len(got) != 0 {
		t.Fatalf("clean warnings = %q", got)
	}
	n := apiclient.Negotiation{Handshake: wire.Handshake{TCPAddr: "127.0.0.1:1"}, Hint: "server is an older build"}
	all := strings.Join(statusWarnings(n, remote), "\n")
	for _, want := range []string{"TCP listener is on", "older build"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in %q", want, all)
		}
	}
}

func TestWaitForExit_WaitsUntilPidIsGone(t *testing.T) {
	checks := 0
	alive := func(int) bool { checks++; return checks < 3 }
	if err := waitForExit(42, time.Now().Add(time.Second), alive); err != nil {
		t.Fatal(err)
	}
	if checks != 3 {
		t.Fatalf("got %d alive checks, want 3", checks)
	}
}

func TestWaitForExit_PidOutlivesDeadline(t *testing.T) {
	alive := func(int) bool { return true }
	err := waitForExit(42, time.Now().Add(100*time.Millisecond), alive)
	if err == nil || !strings.Contains(err.Error(), "pid 42") {
		t.Fatalf("got %v, want a still-running error", err)
	}
}
