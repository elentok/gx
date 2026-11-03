package server

import (
	"testing"
	"time"
)

// launch hands this ctx to the runner's launch prompt, so a stop reaches an
// in-flight launch only if close ends it.
func TestSessionWatches_CloseCancelsTheLaunchContext(t *testing.T) {
	t.Parallel()
	w := newSessionWatches()
	ctx, release := w.context()
	defer release()

	w.close()

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("launch context still live after close")
	}
}
