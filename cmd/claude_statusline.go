package cmd

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
)

var (
	claudeModelStyle    = lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)
	claudeErrorStyle    = lipgloss.NewStyle().Foreground(ui.ColorRed)
	claudeFaintStyle    = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	claudePlainStyle    = lipgloss.NewStyle().Foreground(ui.ColorText)
	claudeWorktreeStyle = lipgloss.NewStyle().Foreground(ui.ColorMauve)
	claudeBranchStyle   = lipgloss.NewStyle().Foreground(ui.ColorSubtle)
	claudeRemoteStyle   = lipgloss.NewStyle().Foreground(ui.ColorGreen)

	claudeSeparator    = claudeFaintStyle.Render("·")
	claudeLeftBracket  = claudeFaintStyle.Render("[")
	claudeRightBracket = claudeFaintStyle.Render("]")
)

// maxSegmentPartLen truncates each of worktree/branch independently so a long
// branch name can't crowd out the worktree name or vice versa.
const maxSegmentPartLen = 24

type statusField struct {
	text    string
	invalid bool
}

type statusLineData struct {
	ContextWindow struct {
		TotalInputTokens json.RawMessage `json:"total_input_tokens"`
		UsedPercentage   json.RawMessage `json:"used_percentage"`
	} `json:"context_window"`
	RateLimits struct {
		FiveHour struct {
			UsedPercentage json.RawMessage `json:"used_percentage"`
			ResetsAt       json.RawMessage `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay struct {
			UsedPercentage json.RawMessage `json:"used_percentage"`
			ResetsAt       json.RawMessage `json:"resets_at"`
		} `json:"seven_day"`
	} `json:"rate_limits"`
	Model struct {
		DisplayName json.RawMessage `json:"display_name"`
	} `json:"model"`
	Remote *struct {
		SessionID json.RawMessage `json:"session_id"`
	} `json:"remote"`
	Vim struct {
		Mode json.RawMessage `json:"mode"`
	} `json:"vim"`
}

// runClaudeStatusline reads a Claude Code statusLine-hook JSON payload from
// d.stdin and prints the rendered status line to d.stdout. silent omits
// missing/invalid fields instead of rendering an error segment; demo prints
// sample output instead of reading stdin.
func runClaudeStatusline(d deps, silent, demo bool) error {
	worktreeSeg := currentWorktreeSegment(d)

	if demo {
		now := time.Now()
		fiveHourResetsAt := now.Add(2 * time.Hour)
		weekResetsAt := now.AddDate(0, 0, 3)
		for _, row := range []struct {
			tokens  float64
			percent int
			remote  bool
			vimMode string
		}{
			{50000, 10, false, "NORMAL"},
			{85000, 30, false, "VISUAL"},
			{120000, 60, false, "INSERT"},
			{50000, 10, true, "REPLACE"},
		} {
			fmt.Fprintln(d.stdout, statusLineFromValues("TheModel", row.tokens, float64(row.percent), 12, 34, fiveHourResetsAt, weekResetsAt, worktreeSeg, row.remote, row.vimMode))
		}
		return nil
	}

	data, err := io.ReadAll(d.stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	var payload statusLineData
	if err := json.Unmarshal(data, &payload); err != nil {
		fmt.Fprintf(d.stdout, "%s\n", claudeErrorStyle.Render("error: malformed JSON input"))
		return fmt.Errorf("parse JSON: %w", err)
	}

	rawTokens, _ := parseNumber(payload.ContextWindow.TotalInputTokens)
	modelText := parseStringField(payload.Model.DisplayName, "model", silent)
	tokensText := parseNumberField(payload.ContextWindow.TotalInputTokens, "tokens", false, silent)
	ctxText := parseContextProgressField(payload.ContextWindow.UsedPercentage, rawTokens, silent)
	// Rate limits are always rendered silently: a hook payload predating this
	// field (or a plan without weekly limits) shouldn't produce error segments.
	fiveText := parseNumberField(payload.RateLimits.FiveHour.UsedPercentage, "5h", true, true)
	weekText := parseNumberField(payload.RateLimits.SevenDay.UsedPercentage, "weekly", true, true)
	fiveResetText := parseResetTimeField(payload.RateLimits.FiveHour.ResetsAt)
	weekResetText := parseResetTimeField(payload.RateLimits.SevenDay.ResetsAt)
	remoteSeg := remoteControlSegment(payload.Remote != nil)
	vimSeg := vimModeSegment(payload.Vim.Mode)

	fmt.Fprintln(d.stdout, statusLineFromParts(rawTokens, modelText, tokensText, ctxText, fiveText, weekText, fiveResetText, weekResetText, worktreeSeg, remoteSeg, vimSeg))
	return nil
}

// claudeVimModeColors maps each Claude Code vim mode to a fixed color so the
// mode is recognizable at a glance. Unknown modes fall back to a neutral
// color rather than being hidden, in case Claude Code adds new modes.
var claudeVimModeColors = map[string]color.Color{
	"NORMAL":  ui.ColorGray,
	"VISUAL":  ui.ColorMagenta,
	"INSERT":  ui.ColorGreen,
	"REPLACE": ui.ColorOrange,
}

// vimModeWidth is the length of the longest known vim mode name ("REPLACE").
// Modes are padded to this width so the status line doesn't jump around as
// the mode changes.
const vimModeWidth = 7

// vimModeIcon is the Nerd Font vim glyph (nf-dev-vim) shown ahead of the mode
// name.
const vimModeIcon = ""

// vimModeSegment renders the current vim mode (e.g. " NORMAL") when the
// statusLine payload includes one. Returns "" when absent, matching the
// hook's behavior of omitting the "vim" object outside of vim editor mode.
// The segment is joined with the rest of the status line by the same "·"
// separator used everywhere else, so no separator is added here.
func vimModeSegment(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var mode string
	if err := json.Unmarshal(raw, &mode); err != nil {
		return ""
	}
	mode = strings.ToUpper(strings.TrimSpace(mode))
	if mode == "" {
		return ""
	}

	modeColor, ok := claudeVimModeColors[mode]
	if !ok {
		modeColor = ui.ColorSubtle
	}
	text := fmt.Sprintf("%s %-*s", vimModeIcon, vimModeWidth, mode)
	return lipgloss.NewStyle().Foreground(modeColor).Render(text)
}

// remoteControlSegment renders a short indicator when Claude Code's Remote
// Control is active for this session. The statusLine hook payload includes a
// top-level "remote" object only while a Remote Control session is attached.
func remoteControlSegment(active bool) string {
	if !active {
		return ""
	}
	return claudeRemoteStyle.Render("📡")
}

func errorField(raw json.RawMessage, name string, silent bool) statusField {
	if silent {
		return statusField{}
	}
	if len(raw) == 0 {
		return statusField{text: name + " missing/invalid", invalid: true}
	}
	return statusField{text: name + " missing/invalid: " + string(raw), invalid: true}
}

func parseStringField(raw json.RawMessage, name string, silent bool) statusField {
	if len(raw) == 0 {
		return errorField(raw, name, silent)
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil || strings.TrimSpace(text) == "" {
		if silent {
			return statusField{}
		}
		return errorField(raw, name, silent)
	}
	return statusField{text: text}
}

func parseNumberField(raw json.RawMessage, name string, asPercent bool, silent bool) statusField {
	if len(raw) == 0 {
		return errorField(raw, name, silent)
	}

	value, ok := parseNumber(raw)
	if !ok {
		if silent {
			return statusField{}
		}
		return errorField(raw, name, silent)
	}

	return statusField{text: numberFromValue(value, asPercent)}
}

func parseContextProgressField(raw json.RawMessage, rawTokens float64, silent bool) statusField {
	if len(raw) == 0 {
		return errorField(raw, "context", silent)
	}

	value, ok := parseNumber(raw)
	if !ok {
		if silent {
			return statusField{}
		}
		return errorField(raw, "context", silent)
	}

	return statusField{text: contextProgressValue(value, rawTokens)}
}

// parseResetTimeField parses a resets_at Unix-epoch-seconds field. It's
// always silent (empty string on missing/invalid), matching how rate limits
// are parsed elsewhere: a payload predating this field shouldn't error.
func parseResetTimeField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	value, ok := parseNumber(raw)
	if !ok {
		return ""
	}
	return formatResetTime(time.Unix(int64(value), 0))
}

// formatResetTime renders t as a local 24h time, prefixed with the
// abbreviated weekday only when t falls on a different local date than now.
func formatResetTime(t time.Time) string {
	local := t.Local()
	now := time.Now()
	if local.Year() == now.Year() && local.YearDay() == now.YearDay() {
		return local.Format("15:04")
	}
	return local.Format("Mon 15:04")
}

func parseNumber(raw json.RawMessage) (float64, bool) {
	var num float64
	if err := json.Unmarshal(raw, &num); err == nil {
		if math.IsNaN(num) || math.IsInf(num, 0) {
			return 0, false
		}
		return num, true
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, false
	}

	parsed, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

func statusLineFromValues(model string, tokens, contextPercent, fiveHourPercent, weekPercent float64, fiveHourResetsAt, weekResetsAt time.Time, worktreeSeg string, remote bool, vimMode string) string {
	vimSeg := ""
	if vimMode != "" {
		vimSeg = vimModeSegment(json.RawMessage(strconv.Quote(vimMode)))
	}
	return statusLineFromParts(
		tokens,
		statusField{text: model},
		statusField{text: numberFromValue(tokens, false)},
		statusField{text: contextProgressValue(contextPercent, tokens)},
		statusField{text: numberFromValue(fiveHourPercent, true)},
		statusField{text: numberFromValue(weekPercent, true)},
		formatResetTime(fiveHourResetsAt),
		formatResetTime(weekResetsAt),
		worktreeSeg,
		remoteControlSegment(remote),
		vimSeg,
	)
}

func statusLineFromParts(rawTokens float64, model, tokens, ctx, five, week statusField, fiveReset, weekReset, worktreeSeg, remoteSeg, vimSeg string) string {
	parts := make([]string, 0, 5)
	if vimSeg != "" {
		parts = append(parts, vimSeg)
	}
	if model.text != "" {
		parts = append(parts, styledValue(model, claudeModelStyle))
	}
	if tokens.text != "" {
		icon := tokenIcon(rawTokens)
		tokensSegment := statusField{text: icon + " " + tokens.text, invalid: tokens.invalid}
		dynamicStyle := lipgloss.NewStyle().Foreground(tokenColor(rawTokens))
		parts = append(parts, styledValue(tokensSegment, dynamicStyle))
	}

	usageParts := make([]string, 0, 3)
	if ctx.text != "" {
		usageParts = append(usageParts, styledValue(ctx, claudePlainStyle))
	}
	if five.text != "" {
		text := five.text + " of 5h"
		if fiveReset != "" {
			text += " (resets " + fiveReset + ")"
		}
		usageParts = append(usageParts, styledValue(
			statusField{text: text, invalid: five.invalid}, claudePlainStyle,
		))
	}
	if week.text != "" {
		text := week.text + " of weekly"
		if weekReset != "" {
			text += " (resets " + weekReset + ")"
		}
		usageParts = append(usageParts, styledValue(
			statusField{text: text, invalid: week.invalid}, claudeFaintStyle,
		))
	}
	if len(usageParts) > 0 {
		parts = append(parts, strings.Join(usageParts, " "+claudeSeparator+" "))
	}
	if worktreeSeg != "" {
		parts = append(parts, worktreeSeg)
	}
	if remoteSeg != "" {
		parts = append(parts, remoteSeg)
	}
	return strings.Join(parts, " "+claudeSeparator+" ")
}

func styledValue(value statusField, style lipgloss.Style) string {
	if value.invalid {
		return claudeErrorStyle.Render(value.text)
	}
	return style.Render(value.text)
}

func numberFromValue(value float64, asPercent bool) string {
	formatted := strconv.FormatFloat(value, 'f', 0, 64)
	if asPercent {
		return formatted + "%"
	}
	if value > 1000 {
		withDecimal := strconv.FormatFloat(math.Round(value/100)/10, 'f', 1, 64)
		return strings.TrimSuffix(withDecimal, ".0") + "k"
	}
	return formatted
}

func contextProgressValue(percent, tokens float64) string {
	pct := math.Max(0, math.Min(100, percent))
	barColor := tokenColor(tokens)
	bar := progress.New(
		progress.WithWidth(12),
		progress.WithFillCharacters('■', '·'),
		progress.WithColors(barColor),
		progress.WithoutPercentage(),
	)
	percentText := lipgloss.NewStyle().Foreground(barColor).Render(numberFromValue(percent, true))
	return fmt.Sprintf("%s%s%s %s", claudeLeftBracket, bar.ViewAs(pct/100), claudeRightBracket, percentText)
}

// tokenColor escalates through gx's palette as context usage grows, giving an
// extra warning step (yellow) between blf's original green/orange/red.
func tokenColor(tokens float64) color.Color {
	switch {
	case tokens < 75000:
		return ui.ColorGreen
	case tokens < 100000:
		return ui.ColorYellow
	case tokens < 150000:
		return ui.ColorOrange
	default:
		return ui.ColorRed
	}
}

func tokenIcon(tokens float64) string {
	switch {
	case tokens < 75000:
		return "🙂"
	case tokens < 100000:
		return "🤔"
	default:
		return "🥵"
	}
}

// currentWorktreeSegment resolves the trailing worktree/branch segment for
// the current directory. Any lookup failure (not in a git worktree, git
// commands unavailable, etc.) resolves to "" so the caller omits the segment
// entirely rather than rendering an error.
func currentWorktreeSegment(d deps) string {
	cwd, err := d.getwd()
	if err != nil {
		return ""
	}

	info, err := git.IdentifyDir(cwd)
	if err != nil {
		return ""
	}

	worktreePath := info.WorktreeRoot
	if worktreePath == "" {
		worktreePath = info.Repo.Root
	}

	worktrees, err := git.ListWorktrees(info.Repo)
	if err != nil {
		return ""
	}

	for _, wt := range worktrees {
		if wt.Path == worktreePath {
			return renderWorktreeSegment(filepath.Base(wt.Path), wt.Branch)
		}
	}
	return ""
}

// renderWorktreeSegment renders "worktree" or "worktree on branch" depending
// on whether the branch differs from the worktree name.
func renderWorktreeSegment(worktree, branch string) string {
	if worktree == "" {
		return ""
	}
	seg := claudeWorktreeStyle.Render(truncateSegmentPart(worktree))
	if branch != "" && branch != worktree {
		seg += claudeBranchStyle.Render(" on " + truncateSegmentPart(branch))
	}
	return seg
}

func truncateSegmentPart(s string) string {
	if len(s) <= maxSegmentPartLen {
		return s
	}
	return s[:maxSegmentPartLen-1] + "…"
}
