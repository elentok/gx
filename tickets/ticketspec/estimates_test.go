package ticketspec

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestWork(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		prefix       int
		actual       int
		want         int
		wantApproxim bool
	}{
		{"stamped prefix", 45_000, 70_000, 25_000, false},
		{"no prefix uses fallback", 0, 70_000, 22_000, true},
		{"actual below fallback", 0, 40_000, 0, true},
		{"actual below prefix", 45_000, 30_000, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, approx := Work(tickets.Ticket{PrefixTokens: tt.prefix, ActualContextWindow: tt.actual})
			if got != tt.want || approx != tt.wantApproxim {
				t.Errorf("Work = %d, %v; want %d, %v", got, approx, tt.want, tt.wantApproxim)
			}
		})
	}
}
