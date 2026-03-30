package stage

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

type parsedDiff struct {
	FileHeader []string
	Lines      []string
	Hunks      []parsedHunk
	Changed    []changedLine
}

type parsedHunk struct {
	Header            string
	StartLine         int
	EndLine           int
	OldStart          int
	OldCount          int
	NewStart          int
	NewCount          int
	ChangedLineOffset []int
}

type changedLine struct {
	LineIndex int
	HunkIndex int
	Prefix    byte
	Text      string
	OldLine   int
	NewLine   int
}

func parseUnifiedDiff(raw string) parsedDiff {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	trimmed := strings.TrimSuffix(raw, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return parsedDiff{}
	}
	lines := strings.Split(trimmed, "\n")
	out := parsedDiff{Lines: lines}

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

			out.Hunks = append(out.Hunks, parsedHunk{
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
			cl := changedLine{
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
			cl := changedLine{
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

func buildHunkPatch(parsed parsedDiff, hunkIndex int) (string, error) {
	if hunkIndex < 0 || hunkIndex >= len(parsed.Hunks) {
		return "", fmt.Errorf("invalid hunk index %d", hunkIndex)
	}
	h := parsed.Hunks[hunkIndex]
	if h.StartLine < 0 || h.EndLine >= len(parsed.Lines) || h.EndLine < h.StartLine {
		return "", fmt.Errorf("invalid hunk bounds")
	}

	header := patchFileHeaderFull(parsed.FileHeader)
	if len(header) == 0 {
		return "", fmt.Errorf("diff file header missing")
	}

	var lines []string
	lines = append(lines, header...)
	lines = append(lines, parsed.Lines[h.StartLine:h.EndLine+1]...)
	return strings.Join(lines, "\n") + "\n", nil
}

func buildSingleLinePatch(parsed parsedDiff, changedIndex int) (string, error) {
	if changedIndex < 0 || changedIndex >= len(parsed.Changed) {
		return "", fmt.Errorf("invalid changed line index %d", changedIndex)
	}
	cl := parsed.Changed[changedIndex]
	if cl.HunkIndex < 0 || cl.HunkIndex >= len(parsed.Hunks) {
		return "", fmt.Errorf("invalid hunk index %d", cl.HunkIndex)
	}
	h := parsed.Hunks[cl.HunkIndex]

	header := patchFileHeader(parsed.FileHeader)
	if len(header) == 0 {
		return "", fmt.Errorf("diff file header missing")
	}

	segmentStart, segmentEnd, err := singleLineSegment(parsed, h, cl.LineIndex)
	if err != nil {
		return "", err
	}

	oldLine := h.OldStart
	newLine := h.NewStart
	oldStart := -1
	newStart := -1
	oldCount := 0
	newCount := 0

	kept := make([]string, 0, h.EndLine-h.StartLine)
	for lineIdx := h.StartLine + 1; lineIdx <= h.EndLine && lineIdx < len(parsed.Lines); lineIdx++ {
		line := parsed.Lines[lineIdx]
		if line == "" {
			continue
		}

		prefix := line[0]
		keep := lineIdx >= segmentStart && lineIdx <= segmentEnd
		if prefix == '\\' {
			keep = keep && len(kept) > 0
		}

		if keep {
			if oldStart == -1 {
				oldStart = oldLine
			}
			if newStart == -1 {
				newStart = newLine
			}
			kept = append(kept, line)
			switch prefix {
			case ' ':
				oldCount++
				newCount++
			case '-':
				oldCount++
			case '+':
				newCount++
			}
		}

		switch prefix {
		case ' ':
			oldLine++
			newLine++
		case '-':
			oldLine++
		case '+':
			newLine++
		}
	}

	if len(kept) == 0 {
		return "", fmt.Errorf("selected line not found in hunk")
	}
	if oldStart < 0 {
		oldStart = cl.OldLine
	}
	if newStart < 0 {
		newStart = cl.NewLine
	}
	hunkHeader := fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldStart, oldCount, newStart, newCount)

	lines := append([]string{}, header...)
	lines = append(lines, hunkHeader)
	lines = append(lines, kept...)
	return strings.Join(lines, "\n") + "\n", nil
}

func buildLineRangePatch(parsed parsedDiff, startChanged, endChanged int) (string, error) {
	if startChanged < 0 || endChanged < 0 || startChanged >= len(parsed.Changed) || endChanged >= len(parsed.Changed) {
		return "", fmt.Errorf("invalid changed line range %d..%d", startChanged, endChanged)
	}
	if startChanged > endChanged {
		startChanged, endChanged = endChanged, startChanged
	}

	header := patchFileHeader(parsed.FileHeader)
	if len(header) == 0 {
		return "", fmt.Errorf("diff file header missing")
	}

	selectedByHunk := map[int]map[int]bool{}
	for i := startChanged; i <= endChanged; i++ {
		cl := parsed.Changed[i]
		if cl.HunkIndex < 0 || cl.HunkIndex >= len(parsed.Hunks) {
			continue
		}
		if selectedByHunk[cl.HunkIndex] == nil {
			selectedByHunk[cl.HunkIndex] = map[int]bool{}
		}
		selectedByHunk[cl.HunkIndex][cl.LineIndex] = true
	}
	if len(selectedByHunk) == 0 {
		return "", fmt.Errorf("selected range has no changed lines")
	}

	out := append([]string{}, header...)

	for hunkIdx, selected := range selectedByHunk {
		h := parsed.Hunks[hunkIdx]

		segments := make([][2]int, 0, len(selected))
		for lineIdx := range selected {
			segStart, segEnd, err := singleLineSegment(parsed, h, lineIdx)
			if err != nil {
				return "", err
			}
			segments = append(segments, [2]int{segStart, segEnd})
		}
		if len(segments) == 0 {
			continue
		}
		sort.Slice(segments, func(i, j int) bool { return segments[i][0] < segments[j][0] })
		merged := make([][2]int, 0, len(segments))
		for _, seg := range segments {
			if len(merged) == 0 || seg[0] > merged[len(merged)-1][1]+1 {
				merged = append(merged, seg)
				continue
			}
			if seg[1] > merged[len(merged)-1][1] {
				merged[len(merged)-1][1] = seg[1]
			}
		}

		oldLine := h.OldStart
		newLine := h.NewStart
		oldStart := -1
		newStart := -1
		oldCount := 0
		newCount := 0
		kept := make([]string, 0, h.EndLine-h.StartLine)

		segmentIdx := 0
		for lineIdx := h.StartLine + 1; lineIdx <= h.EndLine && lineIdx < len(parsed.Lines); lineIdx++ {
			line := parsed.Lines[lineIdx]
			if line == "" {
				continue
			}
			for segmentIdx < len(merged) && lineIdx > merged[segmentIdx][1] {
				segmentIdx++
			}
			keep := segmentIdx < len(merged) && lineIdx >= merged[segmentIdx][0] && lineIdx <= merged[segmentIdx][1]
			prefix := line[0]
			if prefix == '\\' {
				keep = keep && len(kept) > 0
			}

			if keep {
				if oldStart == -1 {
					oldStart = oldLine
				}
				if newStart == -1 {
					newStart = newLine
				}
				kept = append(kept, line)
				switch prefix {
				case ' ':
					oldCount++
					newCount++
				case '-':
					oldCount++
				case '+':
					newCount++
				}
			}

			switch prefix {
			case ' ':
				oldLine++
				newLine++
			case '-':
				oldLine++
			case '+':
				newLine++
			}
		}

		if len(kept) == 0 {
			continue
		}
		if oldStart < 0 {
			oldStart = h.OldStart
		}
		if newStart < 0 {
			newStart = h.NewStart
		}
		hunkHeader := fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldStart, oldCount, newStart, newCount)
		out = append(out, hunkHeader)
		out = append(out, kept...)
	}

	if len(out) == len(header) {
		return "", fmt.Errorf("selected range has no patchable lines")
	}

	return strings.Join(out, "\n") + "\n", nil
}

func singleLineSegment(parsed parsedDiff, h parsedHunk, selectedLine int) (start int, end int, err error) {
	if selectedLine < h.StartLine || selectedLine > h.EndLine {
		return 0, 0, fmt.Errorf("selected line out of hunk")
	}

	start = selectedLine
	for i := selectedLine - 1; i > h.StartLine; i-- {
		line := parsed.Lines[i]
		if line == "" {
			continue
		}
		if line[0] != ' ' {
			break
		}
		start = i
	}

	end = selectedLine
	for i := selectedLine + 1; i <= h.EndLine && i < len(parsed.Lines); i++ {
		line := parsed.Lines[i]
		if line == "" {
			continue
		}
		if line[0] != ' ' {
			break
		}
		end = i
	}

	return start, end, nil
}

func patchFileHeader(fileHeader []string) []string {
	plusPath := ""
	for _, line := range fileHeader {
		if strings.HasPrefix(line, "+++ b/") {
			plusPath = strings.TrimPrefix(line, "+++ b/")
			break
		}
	}

	header := make([]string, 0, len(fileHeader))
	for _, line := range fileHeader {
		if strings.HasPrefix(line, "new file mode ") || strings.HasPrefix(line, "deleted file mode ") {
			continue
		}
		if line == "--- /dev/null" && plusPath != "" {
			line = "--- a/" + plusPath
		}
		if strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			header = append(header, line)
		}
	}
	return header
}

func patchFileHeaderFull(fileHeader []string) []string {
	if len(fileHeader) == 0 {
		return nil
	}
	header := make([]string, 0, len(fileHeader))
	for _, line := range fileHeader {
		header = append(header, line)
	}
	return header
}
