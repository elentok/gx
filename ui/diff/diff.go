package diff

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/elentok/gx/ui"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
var ansiCSIRe = regexp.MustCompile(`\x1b\[[0-9:;<=>?]*[ -/]*[@-~]`)
var ansiOSCRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`) // OSC ... BEL/ST

type RowKind int

const (
	RowPlain RowKind = iota
	RowAdded
	RowRemoved
	RowHunkHeader
)

type ParsedDiff struct {
	FileHeader []string
	Lines      []string
	Hunks      []ParsedHunk
	Changed    []ChangedLine
}

type ParsedHunk struct {
	Header            string
	StartLine         int
	EndLine           int
	OldStart          int
	OldCount          int
	NewStart          int
	NewCount          int
	ChangedLineOffset []int
}

type ChangedLine struct {
	LineIndex int
	HunkIndex int
	Prefix    byte
	Text      string
	OldLine   int
	NewLine   int
}

type SymlinkDiffInfo struct {
	IsSymlink    bool
	WasSymlink   bool
	IsNowSymlink bool
	TypeChange   bool
	OldTarget    string
	NewTarget    string
}

func ParseUnifiedDiff(raw string) ParsedDiff {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	trimmed := strings.TrimSuffix(raw, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return ParsedDiff{}
	}
	lines := strings.Split(trimmed, "\n")
	out := ParsedDiff{Lines: lines}

	firstHunk := -1
	for i, line := range lines {
		if hunkHeaderRE.MatchString(line) {
			firstHunk = i
			break
		}
	}
	if firstHunk == -1 {
		out.FileHeader = append(out.FileHeader, lines...)
		return out
	}
	out.FileHeader = append(out.FileHeader, lines[:firstHunk]...)

	current := -1
	oldLine := 0
	newLine := 0

	for i, line := range lines {
		if m := hunkHeaderRE.FindStringSubmatch(line); m != nil {
			if current >= 0 {
				out.Hunks[current].EndLine = i - 1
			}
			oldStart, _ := strconv.Atoi(m[1])
			oldCount := 1
			if m[2] != "" {
				oldCount, _ = strconv.Atoi(m[2])
			}
			newStart, _ := strconv.Atoi(m[3])
			newCount := 1
			if m[4] != "" {
				newCount, _ = strconv.Atoi(m[4])
			}

			out.Hunks = append(out.Hunks, ParsedHunk{
				Header:    line,
				StartLine: i,
				OldStart:  oldStart,
				OldCount:  oldCount,
				NewStart:  newStart,
				NewCount:  newCount,
			})
			current = len(out.Hunks) - 1
			oldLine = oldStart
			newLine = newStart
			continue
		}

		if current < 0 || line == "" {
			continue
		}

		switch line[0] {
		case ' ':
			oldLine++
			newLine++
		case '-':
			cl := ChangedLine{
				LineIndex: i,
				HunkIndex: current,
				Prefix:    '-',
				Text:      line,
				OldLine:   oldLine,
				NewLine:   newLine,
			}
			out.Changed = append(out.Changed, cl)
			h := &out.Hunks[current]
			h.ChangedLineOffset = append(h.ChangedLineOffset, len(out.Changed)-1)
			oldLine++
		case '+':
			cl := ChangedLine{
				LineIndex: i,
				HunkIndex: current,
				Prefix:    '+',
				Text:      line,
				OldLine:   oldLine,
				NewLine:   newLine,
			}
			out.Changed = append(out.Changed, cl)
			h := &out.Hunks[current]
			h.ChangedLineOffset = append(h.ChangedLineOffset, len(out.Changed)-1)
			newLine++
		case '\\':
			// "\ No newline at end of file"
		default:
			// Headers and other metadata inside diff output.
		}
	}

	if current >= 0 {
		out.Hunks[current].EndLine = len(lines) - 1
	}

	return out
}

func BuildDisplayBaseLines(parsed ParsedDiff, colorLines []string) (lines []string, kinds []RowKind, displayToRaw []int) {
	if len(parsed.Lines) == 0 {
		return nil, nil, nil
	}

	if si := ParseSymlinkDiffInfo(parsed); si.IsSymlink {
		if summary := si.summary(); summary != "" {
			symlinkStyle := lipgloss.NewStyle().Foreground(ui.ColorBlue).Bold(true)
			lines = append(lines, symlinkStyle.Render("  "+summary))
			kinds = append(kinds, RowPlain)
			displayToRaw = append(displayToRaw, -1)
		}
	}

	hdrStyle := lipgloss.NewStyle().Background(ui.ColorSurface).Foreground(ui.ColorText).Bold(true)
	for hi, h := range parsed.Hunks {
		if hi > 0 {
			lines = append(lines, "")
			kinds = append(kinds, RowPlain)
			displayToRaw = append(displayToRaw, -1)
		}

		header := cleanHunkHeader(parsed.Lines[h.StartLine])
		lines = append(lines, hdrStyle.Render(" "+header+" "))
		kinds = append(kinds, RowHunkHeader)
		displayToRaw = append(displayToRaw, h.StartLine)

		for rawIdx := h.StartLine + 1; rawIdx <= h.EndLine && rawIdx < len(parsed.Lines); rawIdx++ {
			line := parsed.Lines[rawIdx]
			if rawIdx < len(colorLines) {
				line = sanitizeANSIInline(colorLines[rawIdx])
			}
			kind := RowPlain
			if len(parsed.Lines[rawIdx]) > 0 {
				switch parsed.Lines[rawIdx][0] {
				case '+':
					kind = RowAdded
					line = stripUnifiedVisibleMarker(line, '+')
				case '-':
					kind = RowRemoved
					line = stripUnifiedVisibleMarker(line, '-')
				}
			}
			lines = append(lines, line)
			kinds = append(kinds, kind)
			displayToRaw = append(displayToRaw, rawIdx)
		}
	}
	return lines, kinds, displayToRaw
}

func DiffBodyPadding(kind RowKind, width int) string {
	if width <= 0 {
		return ""
	}
	spaces := strings.Repeat(" ", width)
	switch kind {
	case RowAdded:
		return lipgloss.NewStyle().Background(lipgloss.Color("#2c3239")).Render(spaces)
	case RowRemoved:
		return lipgloss.NewStyle().Background(lipgloss.Color("#34293a")).Render(spaces)
	default:
		return spaces
	}
}

func HasBinaryDiff(parsed ParsedDiff) bool {
	for _, line := range parsed.Lines {
		if strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch") {
			return true
		}
	}
	return false
}

func ParseSymlinkDiffInfo(parsed ParsedDiff) SymlinkDiffInfo {
	var info SymlinkDiffInfo
	var hasNonSymlinkMode bool

	for _, line := range parsed.Lines {
		switch {
		case strings.HasPrefix(line, "new file mode "):
			mode := strings.TrimSpace(strings.TrimPrefix(line, "new file mode "))
			if mode == "120000" {
				info.IsNowSymlink = true
			} else {
				hasNonSymlinkMode = true
			}
		case strings.HasPrefix(line, "deleted file mode "):
			mode := strings.TrimSpace(strings.TrimPrefix(line, "deleted file mode "))
			if mode == "120000" {
				info.WasSymlink = true
			} else {
				hasNonSymlinkMode = true
			}
		case strings.HasPrefix(line, "old mode "):
			mode := strings.TrimSpace(strings.TrimPrefix(line, "old mode "))
			if mode == "120000" {
				info.WasSymlink = true
			} else {
				hasNonSymlinkMode = true
			}
		case strings.HasPrefix(line, "new mode "):
			mode := strings.TrimSpace(strings.TrimPrefix(line, "new mode "))
			if mode == "120000" {
				info.IsNowSymlink = true
			} else {
				hasNonSymlinkMode = true
			}
		case strings.HasPrefix(line, "index "):
			parts := strings.Fields(line)
			if len(parts) == 3 && parts[2] == "120000" {
				info.WasSymlink = true
				info.IsNowSymlink = true
			}
		}
	}
	info.IsSymlink = info.WasSymlink || info.IsNowSymlink
	if !info.IsSymlink {
		return info
	}
	info.TypeChange = hasNonSymlinkMode

	for _, cl := range parsed.Changed {
		if len(cl.Text) < 2 {
			continue
		}
		target := strings.TrimSpace(cl.Text[1:])
		if strings.HasPrefix(target, "-- ") || strings.HasPrefix(target, "++ ") {
			continue
		}
		if cl.Prefix == '-' && info.WasSymlink && info.OldTarget == "" {
			info.OldTarget = target
		} else if cl.Prefix == '+' && info.IsNowSymlink && info.NewTarget == "" {
			info.NewTarget = target
		}
	}
	return info
}

func (si SymlinkDiffInfo) summary() string {
	switch {
	case si.WasSymlink && si.IsNowSymlink:
		switch {
		case si.OldTarget != "" && si.NewTarget != "":
			return "symlink: " + si.OldTarget + " -> " + si.NewTarget
		case si.NewTarget != "":
			return "symlink -> " + si.NewTarget
		case si.OldTarget != "":
			return "symlink: " + si.OldTarget + " (removed)"
		default:
			return "symlink"
		}
	case !si.WasSymlink && si.IsNowSymlink && si.TypeChange:
		if si.NewTarget != "" {
			return "symlink -> " + si.NewTarget
		}
		return "symlink"
	case si.WasSymlink && !si.IsNowSymlink && si.TypeChange:
		if si.OldTarget != "" {
			return "symlink: " + si.OldTarget + " (removed)"
		}
		return "symlink removed"
	case si.IsNowSymlink && si.NewTarget != "":
		return "symlink -> " + si.NewTarget
	case si.WasSymlink && si.OldTarget != "":
		return "symlink: " + si.OldTarget + " (removed)"
	default:
		return "symlink"
	}
}

func cleanHunkHeader(line string) string {
	first := strings.Index(line, "@@")
	if first == -1 {
		return strings.TrimSpace(line)
	}
	second := strings.Index(line[first+2:], "@@")
	if second == -1 {
		return strings.TrimSpace(line)
	}
	second = first + 2 + second
	tail := strings.TrimSpace(line[second+2:])
	if tail == "" {
		return "hunk"
	}
	return tail
}

func stripUnifiedVisibleMarker(line string, marker byte) string {
	if line == "" {
		return line
	}
	plain := ansi.Strip(line)
	if plain == "" {
		return line
	}

	visibleIdx := -1
	if len(plain) > 0 && plain[0] == marker {
		visibleIdx = 0
	} else {
		searchFrom := 0
		for {
			sep := strings.Index(plain[searchFrom:], "│")
			if sep < 0 {
				break
			}
			sep += searchFrom
			start := sep + len("│")
			for start < len(plain) && plain[start] == ' ' {
				start++
			}
			if start < len(plain) && plain[start] == marker {
				visibleIdx = ansi.StringWidth(plain[:start])
				break
			}
			searchFrom = start
			if searchFrom >= len(plain) {
				break
			}
		}
	}
	if visibleIdx < 0 {
		return line
	}

	total := ansi.StringWidth(line)
	if visibleIdx >= total {
		return line
	}
	before := ansi.Cut(line, 0, visibleIdx)
	after := ansi.Cut(line, visibleIdx+1, total)
	return before + " " + after
}

func sanitizeANSIInline(s string) string {
	s = ansiOSCRe.ReplaceAllString(s, "")
	s = ansiCSIRe.ReplaceAllStringFunc(s, func(seq string) string {
		if strings.HasSuffix(seq, "m") {
			return seq
		}
		return ""
	})
	s = strings.ReplaceAll(s, "\t", "    ")
	b := make([]rune, 0, len(s))
	for _, r := range s {
		if (r < 0x20 && r != 0x1b) || r == 0x7f {
			continue
		}
		b = append(b, r)
	}
	return string(b)
}

func buildRawToDisplayMap(parsed ParsedDiff, displayToRaw []int) []int {
	rawToDisplay := make([]int, len(parsed.Lines))
	for i := range rawToDisplay {
		rawToDisplay[i] = -1
	}
	for i, rawIdx := range displayToRaw {
		if rawIdx >= 0 && rawIdx < len(rawToDisplay) && rawToDisplay[rawIdx] < 0 {
			rawToDisplay[rawIdx] = i
		}
	}
	return rawToDisplay
}

func wrapANSI(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	total := ansi.StringWidth(s)
	if total <= width {
		return []string{s}
	}
	out := make([]string, 0, total/width+1)
	for start := 0; start < total; start += width {
		end := start + width
		if end > total {
			end = total
		}
		part := ansi.Cut(s, start, end)
		if part == "" {
			break
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}
