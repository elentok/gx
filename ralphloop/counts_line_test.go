package ralphloop

import "testing"

// TestRenderCountsLine_DocumentedShapes tables the five shapes ticket 27's
// spec calls out by name: fresh, resumed, already done, empty, and
// mid-run pause.
func TestRenderCountsLine_DocumentedShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		counts EpicCounts
		want   string
	}{
		{
			name:   "fresh start, nothing merged yet",
			counts: EpicCounts{Done: 0, InProgress: 1, Ready: 9, Total: 10},
			want:   "0 done · 1 in progress · 9 ready · 10 total",
		},
		{
			name:   "resumed run reports epic-truth done, not a run-local counter",
			counts: EpicCounts{Done: 6, InProgress: 1, Ready: 3, Total: 10},
			want:   "6 done · 1 in progress · 3 ready · 10 total",
		},
		{
			name:   "already complete",
			counts: EpicCounts{Done: 10, Total: 10},
			want:   "10 done · 10 total",
		},
		{
			name:   "empty epic, no tickets at all",
			counts: EpicCounts{Done: 0, Total: 0},
			want:   "0 done · 0 total",
		},
		{
			name:   "mid-run pause, matching the spec's own example",
			counts: EpicCounts{Done: 8, PausedIdentifiers: []string{"07"}, Ready: 1, Total: 10},
			want:   "8 done · 1 paused: 07 · 1 ready · 10 total",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RenderCountsLine(tt.counts); got != tt.want {
				t.Errorf("RenderCountsLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRenderCountsLine_ZeroSuppression pins that every bucket but done/total
// disappears at zero, and that done/total render even when both are zero.
func TestRenderCountsLine_ZeroSuppression(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		counts EpicCounts
		want   string
	}{
		{"only done and total ever forced", EpicCounts{}, "0 done · 0 total"},
		{"in progress suppressed at zero", EpicCounts{Done: 1, InProgress: 0}, "1 done · 0 total"},
		{"blocked suppressed at zero", EpicCounts{Done: 1, Blocked: 0}, "1 done · 0 total"},
		{"ready suppressed at zero", EpicCounts{Done: 1, Ready: 0}, "1 done · 0 total"},
		{"paused suppressed on empty identifier list", EpicCounts{Done: 1, PausedIdentifiers: nil}, "1 done · 0 total"},
		{
			"every non-forced bucket present at once",
			EpicCounts{Done: 1, InProgress: 2, PausedIdentifiers: []string{"03"}, Blocked: 3, Ready: 4, Total: 11},
			"1 done · 2 in progress · 1 paused: 03 · 3 blocked · 4 ready · 11 total",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RenderCountsLine(tt.counts); got != tt.want {
				t.Errorf("RenderCountsLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRenderCountsLine_PausedIdentifierCap pins that up to five paused
// identifiers list inline in full, and a sixth collapses the rest into an
// overflow marker rather than growing the line unbounded.
func TestRenderCountsLine_PausedIdentifierCap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		identifiers []string
		want        string
	}{
		{
			name:        "exactly five lists in full, no overflow marker",
			identifiers: []string{"01", "02", "03", "04", "05"},
			want:        "0 done · 5 paused: 01, 02, 03, 04, 05 · 0 total",
		},
		{
			name:        "a sixth triggers the overflow marker",
			identifiers: []string{"01", "02", "03", "04", "05", "06"},
			want:        "0 done · 6 paused: 01, 02, 03, 04, 05, +1 more · 0 total",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			counts := EpicCounts{PausedIdentifiers: tt.identifiers}
			if got := RenderCountsLine(counts); got != tt.want {
				t.Errorf("RenderCountsLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
