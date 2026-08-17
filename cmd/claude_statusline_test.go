package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/ui"
)

func TestRunClaudeStatusline_ValidInput(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"context_window":{"total_input_tokens":1234,"used_percentage":51.2},"rate_limits":{"five_hour":{"used_percentage":11.4},"seven_day":{"used_percentage":72.8}},"model":{"display_name":"Claude Sonnet"}}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, false, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"Claude Sonnet", "1.2k", "51%", "11% of 5h", "73% of weekly"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// expectedResetLabel mirrors formatResetTime's same-day-vs-weekday choice so
// the test doesn't assume "same day" and go flaky when a reset time computed
// as now+30m happens to cross local midnight.
func expectedResetLabel(t time.Time) string {
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

func TestRunClaudeStatusline_RateLimitResets(t *testing.T) {
	t.Parallel()
	fiveHourReset := time.Now().Add(30 * time.Minute)
	weekReset := time.Now().AddDate(0, 0, 3)

	out := &strings.Builder{}
	d := deps{
		stdin: strings.NewReader(fmt.Sprintf(
			`{"context_window":{"total_input_tokens":1234,"used_percentage":51.2},"rate_limits":{"five_hour":{"used_percentage":11.4,"resets_at":%d},"seven_day":{"used_percentage":72.8,"resets_at":%d}},"model":{"display_name":"Claude Sonnet"}}`,
			fiveHourReset.Unix(), weekReset.Unix(),
		)),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, false, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"resets " + expectedResetLabel(fiveHourReset),
		"resets " + expectedResetLabel(weekReset),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

func TestRunClaudeStatusline_MissingResetsAtDegradesSilently(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"rate_limits":{"five_hour":{"used_percentage":11},"seven_day":{"used_percentage":72}},"model":{"display_name":"Claude"}}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, false, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "resets") {
		t.Fatalf("expected no resets segment when resets_at absent, got %q", got)
	}
	for _, want := range []string{"11% of 5h", "72% of weekly"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

func TestRunClaudeStatusline_RemoteControlActive(t *testing.T) {
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"model":{"display_name":"Claude"},"remote":{"session_id":"abc123"}}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, true, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}
	if !strings.Contains(out.String(), "📡") {
		t.Fatalf("expected remote control indicator, got %q", out.String())
	}
}

func TestRunClaudeStatusline_RemoteControlInactive(t *testing.T) {
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"model":{"display_name":"Claude"}}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, true, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}
	if strings.Contains(out.String(), "📡") {
		t.Fatalf("expected no remote control indicator, got %q", out.String())
	}
}

func TestRunClaudeStatusline_TokensBelowThreshold(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"context_window":{"total_input_tokens":999,"used_percentage":1},"rate_limits":{"five_hour":{"used_percentage":2},"seven_day":{"used_percentage":3}},"model":{"display_name":"Claude"}}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, false, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}
	if !strings.Contains(out.String(), "999") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunClaudeStatusline_InvalidFields(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"context_window":{"total_input_tokens":"NaN?","used_percentage":"oops"},"rate_limits":{"five_hour":{},"seven_day":{"used_percentage":88}},"model":{"display_name":9}}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, false, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"model missing/invalid: 9",
		`tokens missing/invalid: "NaN?"`,
		`context missing/invalid: "oops"`,
		"88% of weekly",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "5h missing/invalid") || strings.Contains(got, "weekly missing/invalid") {
		t.Fatalf("expected missing 5h/weekly values to be hidden, got %q", got)
	}
}

func TestRunClaudeStatusline_RejectsNonFiniteNumbers(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"context_window":{"total_input_tokens":"NaN","used_percentage":"Infinity"},"rate_limits":{"five_hour":{"used_percentage":10},"seven_day":{"used_percentage":20}},"model":{"display_name":"Claude"}}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, false, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`tokens missing/invalid: "NaN"`,
		`context missing/invalid: "Infinity"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

func TestRunClaudeStatusline_SilentSkipsInvalid(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"context_window":{"total_input_tokens":"bad"},"rate_limits":{"five_hour":{"used_percentage":10}},"model":{"display_name":"Claude"}}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, true, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "missing/invalid") {
		t.Fatalf("unexpected placeholder in silent output: %q", got)
	}
	for _, want := range []string{"Claude", "10% of 5h"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

func TestRunClaudeStatusline_MalformedJSON(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"model":`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, false, false); err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out.String(), "error: malformed JSON input") {
		t.Fatalf("expected malformed JSON message, got %q", out.String())
	}
}

func TestRunClaudeStatusline_Demo(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"model":"ignored"}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, false, true); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}

	// Each demo row now renders as two lines: the main line, then the 5h/weekly
	// usage line.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 8 {
		t.Fatalf("expected 8 lines (4 rows x 2 lines), got %d: %q", len(lines), out.String())
	}
	for i := range 4 {
		mainLine := lines[i*2]
		usageLine := lines[i*2+1]
		if !strings.Contains(mainLine, "TheModel") {
			t.Fatalf("main line %d missing model: %q", i, mainLine)
		}
		for _, want := range []string{"12% of 5h", "34% of weekly"} {
			if !strings.Contains(usageLine, want) {
				t.Fatalf("usage line %d missing %q: %q", i, want, usageLine)
			}
		}
	}
	for i, want := range []string{"NORMAL", "VISUAL", "INSERT", "REPLACE"} {
		if !strings.Contains(lines[i*2], want) {
			t.Fatalf("main line %d missing vim mode %q: %q", i, want, lines[i*2])
		}
	}
	if strings.Contains(lines[0], "📡") || strings.Contains(lines[2], "📡") || strings.Contains(lines[4], "📡") {
		t.Fatalf("did not expect remote control indicator on non-remote demo lines: %q", out.String())
	}
	if !strings.Contains(lines[6], "📡") {
		t.Fatalf("expected remote control indicator on last demo row's main line: %q", lines[6])
	}
	for _, want := range []string{"50k", "85k", "120k", "10%", "30%", "60%", "🙂", "🤔", "🥵", "resets "} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("demo output missing %q: %q", want, out.String())
		}
	}
}

func TestTokenColorThresholds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		tokens float64
		want   any
	}{
		{name: "green below 75k", tokens: 74999, want: ui.ColorGreen},
		{name: "yellow at 75k", tokens: 75000, want: ui.ColorYellow},
		{name: "yellow below 100k", tokens: 99999, want: ui.ColorYellow},
		{name: "orange at 100k", tokens: 100000, want: ui.ColorOrange},
		{name: "orange below 150k", tokens: 149999, want: ui.ColorOrange},
		{name: "red at 150k", tokens: 150000, want: ui.ColorRed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tokenColor(tc.tokens); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestContextProgressValue_RendersBar(t *testing.T) {
	t.Parallel()
	got := contextProgressValue(50, 1000)
	if !strings.Contains(got, "■") && !strings.Contains(got, "·") {
		t.Fatalf("expected a rendered progress bar, got %q", got)
	}
	if !strings.Contains(got, "50%") {
		t.Fatalf("expected the percentage text, got %q", got)
	}
}

func TestCurrentWorktreeSegment_PresentWhenBranchMatchesWorktree(t *testing.T) {
	t.Parallel()
	outer := testutil.TempDotBareRepoWithWorktrees(t, "feature-a")
	d := deps{getwd: func() (string, error) { return filepath.Join(outer, "feature-a"), nil }}

	got := currentWorktreeSegment(d)
	if !strings.Contains(got, "feature-a") {
		t.Fatalf("expected worktree name in segment, got %q", got)
	}
	if strings.Contains(got, " on ") {
		t.Fatalf("expected no branch suffix when branch matches worktree name, got %q", got)
	}
}

func TestCurrentWorktreeSegment_PresentWithBranchWhenDiffers(t *testing.T) {
	t.Parallel()
	outer := testutil.TempDotBareRepoWithWorktrees(t, "feature-b")
	wtDir := filepath.Join(outer, "feature-b")
	testutil.MustGitExported(t, wtDir, "checkout", "-b", "other-branch")

	d := deps{getwd: func() (string, error) { return wtDir, nil }}

	got := currentWorktreeSegment(d)
	if !strings.Contains(got, "feature-b") || !strings.Contains(got, "other-branch") {
		t.Fatalf("expected worktree and branch in segment, got %q", got)
	}
	if !strings.Contains(got, " on ") {
		t.Fatalf("expected an \" on \" separator between worktree and branch, got %q", got)
	}
}

func TestCurrentWorktreeSegment_OmittedOutsideGit(t *testing.T) {
	t.Parallel()
	d := deps{getwd: func() (string, error) { return t.TempDir(), nil }}

	if got := currentWorktreeSegment(d); got != "" {
		t.Fatalf("expected empty segment outside a git worktree, got %q", got)
	}
}

func TestCurrentWorktreeSegment_TruncatesLongNames(t *testing.T) {
	t.Parallel()
	longName := "a-worktree-name-that-is-far-too-long-to-fit"
	outer := testutil.TempDotBareRepoWithWorktrees(t, longName)
	d := deps{getwd: func() (string, error) { return filepath.Join(outer, longName), nil }}

	got := currentWorktreeSegment(d)
	if !strings.Contains(got, "…") {
		t.Fatalf("expected truncation ellipsis in segment, got %q", got)
	}
	if strings.Contains(got, longName) {
		t.Fatalf("expected the long name to be truncated, got %q", got)
	}
}

func TestVimModeSegment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		raw     string
		want    string
		wantAbs bool
	}{
		{name: "normal", raw: `"NORMAL"`, want: "NORMAL "},
		{name: "visual", raw: `"VISUAL"`, want: "VISUAL "},
		{name: "insert", raw: `"INSERT"`, want: "INSERT "},
		{name: "replace", raw: `"REPLACE"`, want: "REPLACE"},
		{name: "lowercase normalized", raw: `"normal"`, want: "NORMAL "},
		{name: "unknown mode still renders", raw: `"SELECT"`, want: "SELECT "},
		{name: "missing", raw: ``, wantAbs: true},
		{name: "empty string", raw: `""`, wantAbs: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := vimModeSegment(json.RawMessage(tc.raw))
			if tc.wantAbs {
				if got != "" {
					t.Fatalf("expected empty segment, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) || !strings.Contains(got, vimModeIcon) {
				t.Fatalf("expected segment to contain %q and the vim icon, got %q", tc.want, got)
			}
		})
	}
}

func TestRunClaudeStatusline_IncludesVimModeSegment(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"model":{"display_name":"Claude"},"vim":{"mode":"INSERT"}}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, false, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}
	if !strings.Contains(out.String(), "INSERT") {
		t.Fatalf("expected vim mode segment, got %q", out.String())
	}
}

func TestRunClaudeStatusline_OmitsVimModeSegmentWhenAbsent(t *testing.T) {
	t.Parallel()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"model":{"display_name":"Claude"}}`),
		stdout: out,
		getwd:  func() (string, error) { return t.TempDir(), nil },
	}

	if err := runClaudeStatusline(d, false, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}
	if strings.Contains(out.String(), vimModeIcon) {
		t.Fatalf("expected no vim mode segment, got %q", out.String())
	}
}

func TestRunClaudeStatusline_IncludesWorktreeSegment(t *testing.T) {
	t.Parallel()
	outer := testutil.TempDotBareRepoWithWorktrees(t, "feature-c")
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(`{"model":{"display_name":"Claude"}}`),
		stdout: out,
		getwd:  func() (string, error) { return filepath.Join(outer, "feature-c"), nil },
	}

	if err := runClaudeStatusline(d, false, false); err != nil {
		t.Fatalf("runClaudeStatusline returned error: %v", err)
	}
	if !strings.Contains(out.String(), "feature-c") {
		t.Fatalf("expected worktree segment in output, got %q", out.String())
	}
}
