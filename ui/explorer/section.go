package explorer

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/ui/diff"
)

var deltaHunkHeaderRe = regexp.MustCompile(`^\s*(?:[•*]\s+)?[^:]+:\d+:(?:\s.*)?$`)
var deltaSideBySideLineRe = regexp.MustCompile(`^\s*│\s*([0-9]+)?\s*│.*│\s*([0-9]+)?\s*│`)

type SideBySideMapping struct {
	DisplayToRaw     []int
	RawToDisplay     []int
	ChangedDisplay   []int
	HunkDisplayRange [][2]int
}

func SplitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func IsDeltaSectionDivider(plain string) bool {
	if plain == "" {
		return false
	}
	for _, r := range plain {
		if r != '─' && r != '-' {
			return false
		}
	}
	return true
}

func BuildSideBySideMapping(parsed diff.ParsedDiff, viewLines []string) SideBySideMapping {
	displayToRaw := make([]int, len(viewLines))
	for i := range displayToRaw {
		displayToRaw[i] = -1
	}
	changedDisplay := make([]int, len(parsed.Changed))
	for i := range changedDisplay {
		changedDisplay[i] = -1
	}

	oldByLine := map[int][]int{}
	newByLine := map[int][]int{}
	for i, cl := range parsed.Changed {
		if cl.Prefix == '-' {
			oldByLine[cl.OldLine] = append(oldByLine[cl.OldLine], i)
		}
		if cl.Prefix == '+' {
			newByLine[cl.NewLine] = append(newByLine[cl.NewLine], i)
		}
	}

	for displayIdx, line := range viewLines {
		plain := ansi.Strip(line)
		m := deltaSideBySideLineRe.FindStringSubmatch(plain)
		if m == nil {
			continue
		}
		left := parseOptionalLineNumber(m[1])
		right := parseOptionalLineNumber(m[2])

		if left > 0 {
			if queue := oldByLine[left]; len(queue) > 0 {
				idx := queue[0]
				oldByLine[left] = queue[1:]
				changedDisplay[idx] = displayIdx
				displayToRaw[displayIdx] = parsed.Changed[idx].LineIndex
			}
		}
		if right > 0 {
			if queue := newByLine[right]; len(queue) > 0 {
				idx := queue[0]
				newByLine[right] = queue[1:]
				changedDisplay[idx] = displayIdx
				if displayToRaw[displayIdx] < 0 {
					displayToRaw[displayIdx] = parsed.Changed[idx].LineIndex
				}
			}
		}
	}

	return SideBySideMapping{
		DisplayToRaw:     displayToRaw,
		RawToDisplay:     diff.BuildRawToDisplayMap(parsed, displayToRaw),
		ChangedDisplay:   changedDisplay,
		HunkDisplayRange: sideBySideHunkDisplayRanges(viewLines, len(parsed.Hunks)),
	}
}

func parseOptionalLineNumber(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0
	}
	return n
}

func sideBySideHunkDisplayRanges(lines []string, hunkCount int) [][2]int {
	if hunkCount <= 0 || len(lines) == 0 {
		return nil
	}
	headers := make([]int, 0, hunkCount)
	for i, line := range lines {
		plain := strings.TrimSpace(ansi.Strip(line))
		if deltaHunkHeaderRe.MatchString(plain) {
			headers = append(headers, i)
		}
	}
	if len(headers) != hunkCount {
		return nil
	}
	ranges := make([][2]int, 0, hunkCount)
	for i, start := range headers {
		end := len(lines) - 1
		if i+1 < len(headers) {
			end = headers[i+1] - 1
		}
		for end >= start {
			plain := strings.TrimSpace(ansi.Strip(lines[end]))
			if plain == "" || IsDeltaSectionDivider(plain) {
				end--
				continue
			}
			break
		}
		for start <= end {
			plain := strings.TrimSpace(ansi.Strip(lines[start]))
			if IsDeltaSectionDivider(plain) {
				start++
				continue
			}
			break
		}
		if end < start {
			end = start
		}
		ranges = append(ranges, [2]int{start, end})
	}
	return ranges
}
