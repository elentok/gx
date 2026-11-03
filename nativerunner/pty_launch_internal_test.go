package nativerunner

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A server stop cancels the launch ctx while the launch waits for the turn.
// The wait polls cancelled(), so a cancelled ctx must end it at once instead of
// after the prompt grace.
func TestPtyPrompter_CancelledCtxEndsTheTurnWait(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pr := &ptyPrompter{ctx: ctx}

	if err := pr.cancelled(); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled() = %v; want context.Canceled", err)
	}
	// waitTurn checks cancelled before it reads the holder, so no holder is needed.
	start := time.Now()
	_, err := holderAnswerer{}.waitTurn(0, time.Minute, pr.cancelled)
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("waitTurn = %v after %v; want context.Canceled at once", err, time.Since(start))
	}
}

func TestPtyPrompter_NilCtxNeverCancels(t *testing.T) {
	t.Parallel()
	if err := (&ptyPrompter{}).cancelled(); err != nil {
		t.Fatalf("cancelled() = %v; want nil", err)
	}
}
