package components

import (
	"strings"
	"testing"
	"time"
)

func TestCommandRunnerStreamsAndWaits(t *testing.T) {
	r := NewCommandRunner(".", "sh", "-c", "printf 'one'; sleep 0.1; printf ' two'")
	r.Start()

	deadline := time.Now().Add(2 * time.Second)
	seen := ""
	for time.Now().Before(deadline) {
		seen += r.Consume()
		if strings.Contains(seen, "one") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(seen, "one") {
		t.Fatalf("expected early streamed output, got %q", seen)
	}

	if err := r.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	seen += r.Consume()
	if !strings.Contains(seen, "one two") {
		t.Fatalf("expected full output, got %q", seen)
	}
}

func TestCommandRunnerCancel(t *testing.T) {
	r := NewCommandRunner(".", "sh", "-c", "sleep 5")
	r.Start()
	time.Sleep(80 * time.Millisecond)
	r.Cancel()

	if err := r.Wait(); err == nil {
		t.Fatalf("expected cancelled command to return error")
	}
}
