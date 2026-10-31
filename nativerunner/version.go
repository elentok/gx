package nativerunner

import (
	"strconv"
	"strings"
)

// CompareVersions compares dotted numeric versions; a non-numeric part
// counts as 0.
func CompareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		na, nb := versionPart(pa, i), versionPart(pb, i)
		if na != nb {
			if na < nb {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionPart(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, _ := strconv.Atoi(parts[i])
	return n
}
