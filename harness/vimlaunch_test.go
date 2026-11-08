package harness_test

import (
	"bytes"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/harness"
)

// launchIO serves next(read number) on each read and logs to buf.
func launchIO(session string, buf *bytes.Buffer, reads *int, next func(n int) []string) harness.TypingIO {
	return harness.TypingIO{
		Rows: func() ([]string, error) {
			*reads++
			return next(*reads), nil
		},
		SendKeys: func(string) error { return nil },
		Session:  session,
		Log:      slog.New(slog.NewTextHandler(buf, nil)),
	}
}

func fixed(rows []string) func(int) []string { return func(int) []string { return rows } }

func TestVimLaunch(t *testing.T) {
	tests := []struct {
		name      string
		overrides bool
		next      func(int) []string
		want      bool
		wantReads int
		wantLog   string
	}{
		{name: "no footer: healthy launch, one settled read", overrides: true,
			next: fixed(inputRows("", "? for shortcuts")), want: false, wantReads: 2},
		{name: "INSERT footer trips", overrides: true,
			next: fixed(inputRows("", "-- INSERT --")), want: true, wantReads: 2, wantLog: "-- INSERT --"},
		{name: "NORMAL footer trips", overrides: true,
			next: fixed(inputRows("", "-- NORMAL --")), want: true, wantReads: 2, wantLog: "-- NORMAL --"},
		{name: "override not applied: no read", overrides: false,
			next: fixed(inputRows("", "-- NORMAL --")), want: false, wantReads: 0},
		{name: "screen never settles: passes and logs", overrides: true,
			next: func(n int) []string { return inputRows(strconv.Itoa(n), "? for shortcuts") }, want: false, wantLog: "never settled"},
		{name: "no input box: false", overrides: true,
			next: fixed([]string{"loading", "-- NORMAL --"}), want: false, wantLog: "never settled"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := "vim-launch-" + strconv.Itoa(i)
			if tt.overrides {
				harness.NoteEditorOverride(session, nil)
			}
			var buf bytes.Buffer
			reads := 0
			if got := harness.CheckVimLaunch(launchIO(session, &buf, &reads, tt.next)); got != tt.want {
				t.Errorf("CheckVimLaunch = %v, want %v", got, tt.want)
			}
			if tt.wantReads > 0 || !tt.overrides {
				if reads != tt.wantReads {
					t.Errorf("reads = %d, want %d", reads, tt.wantReads)
				}
			}
			log := buf.String()
			if tt.wantLog == "" && log != "" {
				t.Errorf("unexpected log: %s", log)
			}
			if tt.wantLog != "" && (!strings.Contains(log, tt.wantLog) || (tt.want && !strings.Contains(log, session))) {
				t.Errorf("log = %q, want footer %q and session %q", log, tt.wantLog, session)
			}
			if n := strings.Count(strings.TrimSpace(log), "\n"); log != "" && n != 0 {
				t.Errorf("want one log line, got %q", log)
			}
		})
	}
}
