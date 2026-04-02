package components

import (
	"strings"
	"testing"
	"time"
)

func TestCommandRunnerStreamsAndWaits(t *testing.T) {
	// Use whole-second sleep for portability across shells/sleep implementations.
	r := NewCommandRunner(".", "sh", "-c", "printf 'one'; sleep 1; printf ' two'")
	r.Start()

	deadline := time.Now().Add(3 * time.Second)
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
	full := seen + r.Consume()
	postDeadline := time.Now().Add(500 * time.Millisecond)
	for !strings.Contains(full, "two") && time.Now().Before(postDeadline) {
		time.Sleep(10 * time.Millisecond)
		full += r.Consume()
	}
	if !strings.Contains(full, "one") || !strings.Contains(full, "two") {
		t.Fatalf("expected full output to include one and two, got %q", full)
	}
	if strings.Index(full, "one") > strings.Index(full, "two") {
		t.Fatalf("expected output order one before two, got %q", full)
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
