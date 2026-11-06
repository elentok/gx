package transcript

import (
	"fmt"
	"regexp"
	"strconv"
)

// savedImageNameRE is the one pattern for a saved Prompt image's file name.
var savedImageNameRE = regexp.MustCompile(`^img-(\d+)-([0-9a-f]{8})\.[a-z]+$`)

// SavedImageName is the name gx gives the n-th saved image of a session:
// img-<n>-<id><ext>. id is 8 lowercase hex digits, ext lowercase with its dot.
func SavedImageName(n int, id, ext string) string {
	return fmt.Sprintf("img-%d-%s%s", n, id, ext)
}

// ParseSavedImageName reports the number and id of a name SavedImageName
// produced; any other name (a marker, an uppercase id or extension) is not one.
func ParseSavedImageName(name string) (n int, id string, ok bool) {
	m := savedImageNameRE.FindStringSubmatch(name)
	if m == nil {
		return 0, "", false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, "", false
	}
	return n, m[2], true
}
