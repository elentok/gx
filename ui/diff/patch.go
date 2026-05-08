package diff

import diffcore "github.com/elentok/gx/ui/diff/core"

func BuildHunkPatch(parsed ParsedDiff, hunkIndex int) (string, error) {
	return diffcore.BuildHunkPatch(parsed, hunkIndex)
}

func BuildSingleLinePatch(parsed ParsedDiff, changedIndex int) (string, error) {
	return diffcore.BuildSingleLinePatch(parsed, changedIndex)
}

func BuildLineRangePatch(parsed ParsedDiff, startChanged, endChanged int) (string, error) {
	return diffcore.BuildLineRangePatch(parsed, startChanged, endChanged)
}
