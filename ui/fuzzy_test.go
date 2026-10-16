package ui

import "testing"

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		fields []string
		want   bool
	}{
		{"empty query matches", "", []string{"anything"}, true},
		{"empty query matches no fields", "", nil, true},
		{"subsequence hit", "fps", []string{"force push safely"}, true},
		{"contiguous hit", "push", []string{"force push"}, true},
		{"order matters", "spf", []string{"force push safely"}, false},
		{"missing rune misses", "fx", []string{"force push"}, false},
		{"longer than field misses", "pushes", []string{"push"}, false},
		{"case-insensitive query", "FP", []string{"force push"}, true},
		{"case-insensitive field", "fp", []string{"Force Push"}, true},
		{"across fields", "pushfirst", []string{"Push", "first one"}, true},
		{"second field alone", "det", []string{"label", "detail"}, true},
		{"no fields misses", "a", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FuzzyMatch(tc.query, tc.fields...); got != tc.want {
				t.Fatalf("FuzzyMatch(%q, %q) = %v, want %v", tc.query, tc.fields, got, tc.want)
			}
		})
	}
}
