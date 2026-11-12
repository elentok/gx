package proccap

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestCap(t *testing.T) {
	t.Parallel()
	ok := func() (int, error) { return 400, nil }
	bad := func() (int, error) { return 0, errors.New("ps unavailable") }
	cases := []struct {
		name     string
		count    func() (int, error)
		headroom int
		want     int
		wantLog  string
	}{
		{"count plus headroom", ok, 1000, 1400, ""},
		{"failed count is uncapped and logged", bad, 1000, 0, "ps unavailable"},
		{"unset headroom is uncapped", ok, 0, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			got := Cap(tc.count, tc.headroom, slog.New(slog.NewTextHandler(&buf, nil)))
			if got != tc.want {
				t.Errorf("cap = %d; want %d", got, tc.want)
			}
			if tc.wantLog == "" && buf.Len() != 0 || !strings.Contains(buf.String(), tc.wantLog) {
				t.Errorf("log = %q; want it to contain %q", buf.String(), tc.wantLog)
			}
		})
	}
}

func TestSource_LimitReadsTheLiveHeadroomAtEachCall(t *testing.T) {
	t.Parallel()
	headroom := 10
	s := Source{Headroom: func() int { return headroom }, Count: func() (int, error) { return 100, nil }}
	if got := s.Limit(nil); got != 110 {
		t.Fatalf("cap = %d; want 110", got)
	}
	headroom = 20
	if got := s.Limit(nil); got != 120 {
		t.Fatalf("cap after a settings change = %d; want 120", got)
	}
	if got := (Source{}).Limit(nil); got != 0 {
		t.Fatalf("cap without headroom = %d; want none", got)
	}
}
