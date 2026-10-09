package herdrrunner

import (
	"testing"
	"time"
)

// ShortenCompactTimings makes confirmCompact's waits test-sized.
func ShortenCompactTimings(t *testing.T) {
	confirm, poll, submit := compactConfirmTimeout, compactSubmitPoll, compactSubmitTimeout
	compactConfirmTimeout, compactSubmitPoll, compactSubmitTimeout = time.Second, 10*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() {
		compactConfirmTimeout, compactSubmitPoll, compactSubmitTimeout = confirm, poll, submit
	})
}
