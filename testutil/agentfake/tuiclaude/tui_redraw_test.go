package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestTUIClaude_RedrawDelayHoldsBackTheScreenAfterAKey(t *testing.T) {
	const delay = 500 * time.Millisecond
	f := startTUIFake(t, tuiLaunchArgs, TUIRedrawDelayEnv+"="+delay.String())
	f.waitFor("pid file", func() bool { return f.pidFile() != nil })

	start := time.Now()
	if _, err := io.WriteString(f.stdin, "zqx"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.screen.String(), "zqx") {
		t.Fatalf("screen shows the typed text before the delay: %q", f.screen.String())
	}
	f.waitScreen("zqx")
	if got := time.Since(start); got < delay {
		t.Fatalf("screen changed after %v, want at least %v", got, delay)
	}
}
