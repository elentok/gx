package ui

import "testing"

func TestProgressSpinnerFillsAndEmptiesAtDocumentedCodepoints(t *testing.T) {
	want := []string{
		"\U000F0A9E", "\U000F0A9F", "\U000F0AA0", "\U000F0AA1",
		"\U000F0AA2", "\U000F0AA3", "\U000F0AA4", "\U000F0AA5",
		"\U000F0AA4", "\U000F0AA3", "\U000F0AA2", "\U000F0AA1",
		"\U000F0AA0", "\U000F0A9F",
	}
	if len(ProgressSpinner.Frames) != len(want) {
		t.Fatalf("expected %d frames, got %d", len(want), len(ProgressSpinner.Frames))
	}
	for i, frame := range want {
		if ProgressSpinner.Frames[i] != frame {
			t.Errorf("frame %d: got %q, want %q", i, ProgressSpinner.Frames[i], frame)
		}
	}
}
