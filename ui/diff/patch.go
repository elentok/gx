package diff

import (
	"fmt"
	"sort"
	"strings"
)

func BuildHunkPatch(parsed ParsedDiff, hunkIndex int) (string, error) {
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

func BuildSingleLinePatch(parsed ParsedDiff, changedIndex int) (string, error) {
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

func BuildLineRangePatch(parsed ParsedDiff, startChanged, endChanged int) (string, error) {
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

func singleLineSegment(parsed ParsedDiff, h ParsedHunk, selectedLine int) (start int, end int, err error) {
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
