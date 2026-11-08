package prompt_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/elentok/gx/server/sessions/prompt"
)

// scripted returns a read function that plays out reads in order, repeating
// the last one; an entry with err set fails.
type scriptedRead struct {
	rows []string
	err  error
}

func scripted(reads ...scriptedRead) func() ([]string, error) {
	i := 0
	return func() ([]string, error) {
		r := reads[min(i, len(reads)-1)]
		i++
		return r.rows, r.err
	}
}

func TestSettleScreen(t *testing.T) {
	t.Parallel()
	a, b := []string{"a"}, []string{"b"}
	boom := errors.New("boom")
	tests := map[string]struct {
		reads []scriptedRead
		want  []string
	}{
		"stable screen returns after two equal reads": {[]scriptedRead{{rows: a}, {rows: a}}, a},
		"screen changing on the second read returns the new rows": {
			[]scriptedRead{{rows: a}, {rows: b}, {rows: b}}, b},
		"read error returns the last rows": {
			[]scriptedRead{{rows: a}, {rows: b}, {err: boom}}, b},
		"first read error returns none": {[]scriptedRead{{err: boom}}, nil},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := prompt.SettleScreen(scripted(tc.reads...), time.Millisecond, time.Second)
			if !slices.Equal(got, tc.want) {
				t.Errorf("rows = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSettleScreen_NeverSettlingReturnsTheLastRowsAtTheCap(t *testing.T) {
	t.Parallel()
	n := 0
	read := func() ([]string, error) {
		n++
		return []string{string(rune('a' + n%26))}, nil
	}
	start := time.Now()
	got := prompt.SettleScreen(read, 5*time.Millisecond, 50*time.Millisecond)
	if len(got) != 1 || got[0] != string(rune('a'+n%26)) {
		t.Errorf("rows = %v after %d reads, want the last one read", got, n)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v, want it to stop near the cap", elapsed)
	}
}
