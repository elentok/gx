package nativerunner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

// fakeclaudeRunner returns a pty runner whose claude is the built fakeclaude
// binary, not this test binary, as an e2e fixture runs it.
func fakeclaudeRunner(t *testing.T) *nativerunner.Pty {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "claude")
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/elentok/gx/testutil/agentfake/cmd/fakeclaude").CombinedOutput(); err != nil {
		t.Fatalf("build fakeclaude: %v\n%s", err, out)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := shortTempDir(t)
	t.Cleanup(func() { killAgents(root) })
	p := ptyOn(root, exe)
	p.Env = ptyTestEnv(t.TempDir(), holderEnv+"=1")
	p.Claude = bin
	return p
}

func TestFakeclaude_PtyReadsScreenAndAnswersPermission(t *testing.T) {
	p := fakeclaudeRunner(t)
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: t.TempDir(), Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop(s) })
	if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second); err != nil {
		t.Fatalf("idle screen not read as idle: %v", err)
	}

	if err := p.Prompt(s, agentfake.PermissionPromptPrefix+"rg foo"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second); err != nil {
		t.Fatalf("permission prompt not read as blocked: %v", err)
	}
	pend := pendingOf(t, p, s)
	if pend == nil || pend.Tool != "Bash command" || pend.Description != "rg foo" {
		t.Fatalf("Pending = %+v; want the Bash command rg foo", pend)
	}

	if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: pend.ID}); err != nil {
		t.Fatal(err)
	}
	if st, err := p.Status(s); err != nil || st.State != agentrunner.StateWorking || st.Pending != nil {
		t.Fatalf("Status = %+v, %v; want working with the prompt cleared", st, err)
	}
	ptyControl(t, p, s, agentfake.Control{Action: agentfake.ActionFinish})
	if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second); err != nil {
		t.Fatalf("program did not continue to idle: %v", err)
	}
}
