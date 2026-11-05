package ralphloop

import (
	"testing"
	"time"
)

// TestGate_WakePaused_ShortensPauseWait: WakePaused cuts a pause wait short
// instead of it running out a long pause-poll-interval wait.
func TestGate_WakePaused_ShortensPauseWait(t *testing.T) {
	t.Parallel()
	gate := NewGate()
	waiting := make(chan struct{})
	woke := make(chan struct{})

	go func() {
		timer := time.After(time.Hour)
		pauseWake := gate.PauseWake()
		close(waiting)
		select {
		case <-timer:
		case <-pauseWake:
		}
		close(woke)
	}()

	<-waiting
	gate.WakePaused()

	select {
	case <-woke:
	case <-time.After(5 * time.Second):
		t.Fatal("WakePaused() did not interrupt an in-progress pause wait")
	}
}
