package diff

import (
	diffcore "github.com/elentok/gx/ui/diff/core"
	diffrender "github.com/elentok/gx/ui/diff/render"
)

type RowKind = diffrender.RowKind

const (
	RowPlain      = diffrender.RowPlain
	RowAdded      = diffrender.RowAdded
	RowRemoved    = diffrender.RowRemoved
	RowHunkHeader = diffrender.RowHunkHeader
)

type ParsedDiff = diffcore.ParsedDiff
type ParsedHunk = diffcore.ParsedHunk
type ChangedLine = diffcore.ChangedLine
type SymlinkDiffInfo = diffrender.SymlinkDiffInfo

func ParseUnifiedDiff(raw string) ParsedDiff {
	return diffcore.ParseUnifiedDiff(raw)
}

func BuildDisplayBaseLines(parsed ParsedDiff, colorLines []string) (lines []string, kinds []RowKind, displayToRaw []int) {
	return diffrender.BuildDisplayBaseLines(parsed, colorLines)
}

func DiffBodyPadding(kind RowKind, width int) string {
	return diffrender.DiffBodyPadding(kind, width)
}

func HasBinaryDiff(parsed ParsedDiff) bool { return diffrender.HasBinaryDiff(parsed) }

func SectionHasBinaryDiff(parsed ParsedDiff) bool { return diffrender.SectionHasBinaryDiff(parsed) }

func ParseSymlinkDiffInfo(parsed ParsedDiff) SymlinkDiffInfo {
	return diffrender.ParseSymlinkDiffInfo(parsed)
}

func CleanHunkHeader(line string) string { return diffrender.CleanHunkHeader(line) }

func StripUnifiedVisibleMarker(line string, marker byte) string {
	return diffrender.StripUnifiedVisibleMarker(line, marker)
}

func SanitizeANSIInline(s string) string { return diffrender.SanitizeANSIInline(s) }

func BuildRawToDisplayMap(parsed ParsedDiff, displayToRaw []int) []int {
	return diffcore.BuildRawToDisplayMap(parsed, displayToRaw)
}

func WrapANSI(s string, width int) []string { return diffrender.WrapANSI(s, width) }
