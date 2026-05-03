package status

import "github.com/elentok/gx/ui/diff"

type parsedDiff = diff.ParsedDiff
type parsedHunk = diff.ParsedHunk
type changedLine = diff.ChangedLine

func parseUnifiedDiff(raw string) parsedDiff { return diff.ParseUnifiedDiff(raw) }
