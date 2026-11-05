package holder_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	agentharness "github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/testutil"
)

// keyChild sits at a permission prompt and records the first key as hex in
// $HOME/key; tail runs after it.
func keyChild(tail string) string {
	return `d="$HOME/.claude/sessions"; mkdir -p "$d"; f="$d/$$.json"; t=$(date +%s)000
st() { printf '{"pid":%s,"startedAt":%s,"status":"%s"%s}' $$ "$t" "$1" "$2" > "$f.tmp" && mv "$f.tmp" "$f"; }
st waiting ',"waitingFor":"permission prompt"'
stty -icanon -icrnl min 1; printf 'ready\n'; dd bs=1 count=1 2>/dev/null | od -An -tx1 > "$HOME/key.tmp"; mv "$HOME/key.tmp" "$HOME/key"
` + tail
}

func startKeyHolder(t *testing.T, tail string) (*holder.HolderClient, string) {
	t.Helper()
	home := shortTempDir(t)
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	_, c, _ := startHolder(t, keyChild(tail))
	waitForScreen(t, c, "ready")
	waitForStatus(t, c, nativerunner.PidStatus{Status: nativerunner.ClaudeWaiting, WaitingFor: "permission prompt"}, 1)
	return c, filepath.Join(home, "key")
}

func keyPressed(t *testing.T, keyFile string) string {
	t.Helper()
	b, err := os.ReadFile(keyFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func TestHolderAnswer_ClaudeAllowSendsEnter(t *testing.T) {
	c, key := startKeyHolder(t, `st idle; cat >/dev/null`)
	if err := holder.AnswerClaude(c, t.TempDir(), agentrunner.Answer{Decision: agentrunner.DecisionAllow}); err != nil {
		t.Fatal(err)
	}
	if got := keyPressed(t, key); got != "0d" {
		t.Errorf("key = %q; want Enter (0d)", got)
	}
}

func TestHolderAnswer_ClaudeDenySendsEsc(t *testing.T) {
	c, key := startKeyHolder(t, `st idle; cat >/dev/null`)
	if err := holder.AnswerClaude(c, t.TempDir(), agentrunner.Answer{Decision: agentrunner.DecisionDeny}); err != nil {
		t.Fatal(err)
	}
	if got := keyPressed(t, key); got != "1b" {
		t.Errorf("key = %q; want Esc (1b)", got)
	}
}

func TestHolderAnswer_ClaudeDialogThatStaysIsNotDelivered(t *testing.T) {
	c, _ := startKeyHolder(t, `cat >/dev/null`)
	err := holder.AnswerClaude(c, t.TempDir(), agentrunner.Answer{Decision: agentrunner.DecisionAllow})
	if !errors.Is(err, agentrunner.ErrNotDelivered) {
		t.Errorf("err = %v; want ErrNotDelivered", err)
	}
}

func TestHolderAnswer_ClaudeRequestMismatchSendsNothing(t *testing.T) {
	c, key := startKeyHolder(t, `st idle; cat >/dev/null`)
	err := holder.AnswerClaude(c, t.TempDir(), agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "other"})
	if !errors.Is(err, agentrunner.ErrRequestMismatch) {
		t.Fatalf("err = %v; want ErrRequestMismatch", err)
	}
	time.Sleep(200 * time.Millisecond)
	if got := keyPressed(t, key); got != "" {
		t.Errorf("key = %q; want none sent", got)
	}
}

// codexHolder runs a Codex-like child that draws screen, records the first key
// and then lets the watcher go idle, as Codex does once the dialog closes.
func codexHolder(t *testing.T, screen string) (c *holder.HolderClient, dir, keyFile string) {
	t.Helper()
	dir = shortTempDir(t)
	keyFile = filepath.Join(dir, "key")
	w := &settableWatcher{}
	w.set(agentharness.Status{State: agentharness.Blocked, Blocked: agentharness.BlockedPermission, Message: "approval"}, "")
	child := `printf '%s\n' "$2"; stty -icanon min 1; dd bs=1 count=1 2>/dev/null | od -An -tx1 > "$1.tmp"; mv "$1.tmp" "$1"; cat >/dev/null`
	c, finished := startTestHolder(t, dir, settableHarness{w: w}, []string{"sh", "-c", child, "sh", keyFile, screen})
	t.Cleanup(func() {
		_ = c.Stop()
		<-finished
	})
	go func() {
		if testutil.Poll(5*time.Second, 10*time.Millisecond, func() bool { _, err := os.Stat(keyFile); return err == nil }) {
			w.set(agentharness.Status{State: agentharness.Idle}, "")
		}
	}()
	return c, dir, keyFile
}

const codexDialog = "Blocked by hook\n  Run rm -rf build"

func TestHolderAnswer_CodexHookWritesTheDecisionFile(t *testing.T) {
	c, dir, _ := codexHolder(t, "nothing readable")
	if err := holder.WritePermissionRequest(dir, holder.PermissionRequest{ID: "r1", Tool: "Bash"}); err != nil {
		t.Fatal(err)
	}
	// A decision file does not move the fake watcher; the answer waits for it.
	go func() {
		testutil.Poll(5*time.Second, 10*time.Millisecond, func() bool {
			_, ok, _ := holder.ReadPermissionDecision(dir)
			return ok
		})
		_ = c.SendKeys("x")
	}()
	err := holder.AnswerCodex(c, dir, agentrunner.Answer{Decision: agentrunner.DecisionDeny, RequestID: "r1", Text: "no"}, false)
	if err != nil {
		t.Fatal(err)
	}
	d, ok, err := holder.ReadPermissionDecision(dir)
	if err != nil || !ok || d.ID != "r1" || d.Allow || d.Reason != "no" {
		t.Errorf("decision = %+v, %v, %v; want a deny of r1 with the reason", d, ok, err)
	}
}

func TestHolderAnswer_CodexHookRequestMismatchWritesNothing(t *testing.T) {
	c, dir, _ := codexHolder(t, "nothing readable")
	if err := holder.WritePermissionRequest(dir, holder.PermissionRequest{ID: "r1", Tool: "Bash"}); err != nil {
		t.Fatal(err)
	}
	err := holder.AnswerCodex(c, dir, agentrunner.Answer{Decision: agentrunner.DecisionAllow, RequestID: "r2"}, false)
	if !errors.Is(err, agentrunner.ErrRequestMismatch) {
		t.Fatalf("err = %v; want ErrRequestMismatch", err)
	}
	if _, ok, _ := holder.ReadPermissionDecision(dir); ok {
		t.Error("a decision was written for a mismatched request")
	}
}

func TestHolderAnswer_CodexKeysAllowSendsY(t *testing.T) {
	c, dir, key := codexHolder(t, codexDialog)
	if err := holder.AnswerCodex(c, dir, agentrunner.Answer{Decision: agentrunner.DecisionAllow}, false); err != nil {
		t.Fatal(err)
	}
	if got := keyPressed(t, key); got != "79" {
		t.Errorf("key = %q; want y (79)", got)
	}
}

func TestHolderAnswer_CodexUnreadableScreenPolicy(t *testing.T) {
	t.Run("queue refuses both", func(t *testing.T) {
		c, dir, key := codexHolder(t, "nothing readable")
		for _, d := range []agentrunner.Decision{agentrunner.DecisionAllow, agentrunner.DecisionDeny} {
			err := holder.AnswerCodex(c, dir, agentrunner.Answer{Decision: d}, false)
			if !errors.Is(err, agentrunner.ErrRequestMismatch) {
				t.Errorf("%s: err = %v; want ErrRequestMismatch", d, err)
			}
		}
		if got := keyPressed(t, key); got != "" {
			t.Errorf("key = %q; want none sent", got)
		}
	})
	t.Run("keys route denies with Esc", func(t *testing.T) {
		c, dir, key := codexHolder(t, "nothing readable")
		if err := holder.AnswerCodex(c, dir, agentrunner.Answer{Decision: agentrunner.DecisionDeny}, true); err != nil {
			t.Fatal(err)
		}
		if got := keyPressed(t, key); got != "1b" {
			t.Errorf("key = %q; want Esc (1b)", got)
		}
	})
	t.Run("keys route refuses Allow", func(t *testing.T) {
		c, dir, key := codexHolder(t, "nothing readable")
		err := holder.AnswerCodex(c, dir, agentrunner.Answer{Decision: agentrunner.DecisionAllow}, true)
		if !errors.Is(err, holder.ErrNotApprovable) {
			t.Errorf("err = %v; want ErrNotApprovable", err)
		}
		if got := keyPressed(t, key); got != "" {
			t.Errorf("key = %q; want none sent", got)
		}
	})
}
