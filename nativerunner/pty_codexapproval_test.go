package nativerunner_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	gxharness "github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

// codexApprovalSession starts a Codex fake that can reach `gx agent
// permission-request`. With hook false the PermissionRequest hook is left out,
// so the fake draws the dialog and takes keys.
func codexApprovalSession(t *testing.T, id string, hook bool) (*nativerunner.Pty, agentrunner.Session, string) {
	t.Helper()
	gxDir := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(gxDir, "gx"), "github.com/elentok/gx").CombinedOutput(); err != nil {
		t.Fatalf("build gx: %v\n%s", err, out)
	}
	p, home := codexRunner(t, agentfake.CodexIDEnv+"="+id, "PATH="+gxDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	args := gxharness.CodexArgs(gxharness.CodexArgsInput{})
	if !hook {
		i := slices.IndexFunc(args, func(a string) bool { return strings.HasPrefix(a, "hooks.PermissionRequest=") })
		args = slices.Delete(args, i-1, i+1)
	}
	s, err := p.Start(agentrunner.StartOptions{Label: "e-01", Epic: "e", Cwd: realDir(t), Kind: gxharness.Codex, Args: args})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop(s) })
	if _, err := p.Wait(s, []agentrunner.State{agentrunner.StateIdle}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	// A deny ends a turn, so there must be one.
	if err := p.Prompt(s, "run it"); err != nil {
		t.Fatal(err)
	}
	return p, s, home
}

func askPermission(t *testing.T, id, command string) {
	t.Helper()
	if err := agentfake.SendProgramControl(id, agentfake.Control{Action: agentfake.ActionPermission, Command: command}); err != nil {
		t.Fatal(err)
	}
}

// awaitPending polls until the blocked session shows a card.
func awaitPending(t *testing.T, p *nativerunner.Pty, s agentrunner.Session) *agentrunner.Pending {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st, err := p.Status(s)
		if err != nil {
			t.Fatal(err)
		}
		if st.State == agentrunner.StateBlocked && st.Pending != nil {
			return st.Pending
		}
		time.Sleep(20 * time.Millisecond)
	}
	if holder, err := nativerunner.DialHolder(filepath.Join(p.Root, "e-01")); err == nil {
		rows, _, _ := holder.Screen(0)
		t.Logf("screen:\n%s", strings.Join(rows, "\n"))
	}
	t.Fatal("no Pending card")
	return nil
}

func TestPtyCodexApproval_HookCardAllowContinuesTheAgent(t *testing.T) {
	t.Parallel()
	p, s, home := codexApprovalSession(t, "perm-allow", true)
	askPermission(t, "perm-allow", "rg foo")

	pending := awaitPending(t, p, s)
	if pending.Tool != "Bash" || pending.Description != "rg foo" || pending.ID == "" {
		t.Fatalf("Pending = %+v, want the Bash tool with its command", pending)
	}
	if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "stale"}); !errors.Is(err, agentrunner.ErrRequestMismatch) {
		t.Fatalf("stale request id: err = %v, want ErrRequestMismatch", err)
	}
	if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: pending.ID}); err != nil {
		t.Fatal(err)
	}
	st, err := p.Wait(s, []agentrunner.State{agentrunner.StateWorking}, 5*time.Second)
	if err != nil || st.Pending != nil {
		t.Fatalf("after allow: %+v, %v", st, err)
	}
	runs, err := agentfake.ReadCodexHookLog(home, "perm-allow")
	if err != nil {
		t.Fatal(err)
	}
	if last := runs[len(runs)-1]; last.Event != "PermissionRequest" || !strings.Contains(last.Stdout, `"allow"`) {
		t.Fatalf("hook run = %+v, want an allow", last)
	}
}

func TestPtyCodexApproval_HookDenyPassesTheReason(t *testing.T) {
	t.Parallel()
	p, s, home := codexApprovalSession(t, "perm-deny", true)
	askPermission(t, "perm-deny", "rg foo")

	pending := awaitPending(t, p, s)
	if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionDeny, RequestID: pending.ID, Text: "use fd instead"}); err != nil {
		t.Fatal(err)
	}
	holder, err := nativerunner.DialHolder(filepath.Join(p.Root, "e-01"))
	if err != nil {
		t.Fatal(err)
	}
	waitForScreen(t, holder, "denied: use fd instead")
	runs, err := agentfake.ReadCodexHookLog(home, "perm-deny")
	if err != nil {
		t.Fatal(err)
	}
	if last := runs[len(runs)-1]; !strings.Contains(last.Stdout, `"deny"`) || !strings.Contains(last.Stdout, "use fd instead") {
		t.Fatalf("hook run = %+v, want a deny with the reason", last)
	}
}

func TestPtyCodexApproval_ScreenDialogWhenTheHookDidNotFire(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		decision agentrunner.Decision
		text     string
		want     string
	}{
		{"allow", agentrunner.DecisionAllow, "", "ran:"},
		{"deny with reason", agentrunner.DecisionDeny, "use fd instead", "use fd instead"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id := "perm-screen-" + strings.ReplaceAll(tc.name, " ", "-")
			p, s, home := codexApprovalSession(t, id, false)
			askPermission(t, id, "rg foo")

			pending := awaitPending(t, p, s)
			if pending.Tool != "Blocked by hook" || pending.Description != "rg foo" {
				t.Fatalf("Pending = %+v, want the dialog's title and message", pending)
			}
			if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "stale"}); !errors.Is(err, agentrunner.ErrRequestMismatch) {
				t.Fatalf("stale request id: err = %v, want ErrRequestMismatch", err)
			}
			if err := p.Answer(s, agentrunner.Answer{Decision: tc.decision, RequestID: pending.ID, Text: tc.text}); err != nil {
				t.Fatal(err)
			}
			if tc.decision == agentrunner.DecisionAllow {
				// The fake resumes its turn and works on; the card is gone.
				if st, err := p.Wait(s, []agentrunner.State{agentrunner.StateWorking}, 5*time.Second); err != nil || st.Pending != nil {
					t.Fatalf("after allow: %+v, %v", st, err)
				}
				return
			}
			// The keys only cancel; the reason reaches the model as its next prompt.
			awaitRollout(t, home, id, tc.want)
		})
	}
}

// awaitRollout polls until the session's rollout under home holds want.
func awaitRollout(t *testing.T, home, id, want string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		found := false
		_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.Contains(d.Name(), id) || !strings.HasSuffix(d.Name(), ".jsonl") {
				return nil
			}
			if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), want) {
				found = true
			}
			return nil
		})
		if found {
			return
		}
	}
	t.Fatalf("the rollout never held %q", want)
}

func TestPtyCodexApproval_UnknownScreenIsNotApprovable(t *testing.T) {
	t.Parallel()
	p, s, _ := codexApprovalSession(t, "perm-unknown", false)
	// Blocked on a permission with nothing on the screen to read.
	if err := agentfake.SendProgramControl("perm-unknown", agentfake.Control{Action: agentfake.ActionReport, State: "blocked", Kind: "permission"}); err != nil {
		t.Fatal(err)
	}
	st, err := p.Wait(s, []agentrunner.State{agentrunner.StateBlocked}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if st.Pending != nil {
		t.Fatalf("Pending = %+v, want none for a screen gx cannot read", st.Pending)
	}
	if err := p.Answer(s, agentrunner.Answer{Decision: agentrunner.DecisionAllow}); !errors.Is(err, agentrunner.ErrRequestMismatch) {
		t.Fatalf("answer on an unknown screen: err = %v, want ErrRequestMismatch", err)
	}
}
