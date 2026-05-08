package diffrender

import (
	"regexp"
	"strings"

	"github.com/elentok/gx/ui"
	diffcore "github.com/elentok/gx/ui/diff/diffcore"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type RowKind int

const (
	RowPlain RowKind = iota
	RowAdded
	RowRemoved
	RowHunkHeader
)

type ParsedDiff = diffcore.ParsedDiff

type SymlinkDiffInfo struct {
	IsSymlink    bool
	WasSymlink   bool
	IsNowSymlink bool
	TypeChange   bool
	OldTarget    string
	NewTarget    string
}

func BuildDisplayBaseLines(parsed ParsedDiff, colorLines []string) (lines []string, kinds []RowKind, displayToRaw []int) {
	if len(parsed.Lines) == 0 {
		return nil, nil, nil
	}

	if si := ParseSymlinkDiffInfo(parsed); si.IsSymlink {
		if summary := si.Summary(); summary != "" {
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

func SectionHasBinaryDiff(parsed ParsedDiff) bool { return HasBinaryDiff(parsed) }

func (si SymlinkDiffInfo) TitleLabel() string {
	switch {
	case si.WasSymlink && si.IsNowSymlink:
		return "[symlink]"
	case !si.WasSymlink && si.IsNowSymlink && si.TypeChange:
		return "[regular -> symlink]"
	case !si.WasSymlink && si.IsNowSymlink && !si.TypeChange:
		return "[symlink]"
	case si.WasSymlink && !si.IsNowSymlink && si.TypeChange:
		return "[symlink -> regular]"
	case si.WasSymlink && !si.IsNowSymlink && !si.TypeChange:
		return "[symlink]"
	default:
		return ""
	}
}

func (si SymlinkDiffInfo) Summary() string {
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
			return "regular file -> symlink (" + si.NewTarget + ")"
		}
		return "regular file -> symlink"
	case si.WasSymlink && !si.IsNowSymlink && si.TypeChange:
		if si.OldTarget != "" {
			return "symlink (" + si.OldTarget + ") -> regular file"
		}
		return "symlink -> regular file"
	case si.IsNowSymlink && si.NewTarget != "":
		return "symlink -> " + si.NewTarget
	case si.WasSymlink && si.OldTarget != "":
		return "symlink: " + si.OldTarget + " (removed)"
	default:
		return ""
	}
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

func CleanHunkHeader(line string) string { return cleanHunkHeader(line) }

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

func StripUnifiedVisibleMarker(line string, marker byte) string {
	return stripUnifiedVisibleMarker(line, marker)
}

var ansiCSIRe = regexp.MustCompile(`\x1b\[[0-9:;<=>?]*[ -/]*[@-~]`)
var ansiOSCRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`) // OSC ... BEL/ST

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

func SanitizeANSIInline(s string) string { return sanitizeANSIInline(s) }

func WrapANSI(s string, width int) []string {
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
