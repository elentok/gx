package files

import (
	"net/url"
	"strings"
)

// PathForm decodes the raw escaped URL path of a raw or token file request
// into an absolute path. It returns false for anything non-canonical.
// Encoded separators are refused before decoding so that "%2F" can never turn
// into a segment boundary.
func PathForm(escaped string) (string, bool) {
	if !strings.HasPrefix(escaped, "/") || strings.Contains(escaped, "//") || strings.Contains(escaped, `\`) {
		return "", false
	}
	lower := strings.ToLower(escaped)
	if strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
		return "", false
	}
	decoded, err := url.PathUnescape(escaped)
	if err != nil || strings.ContainsAny(decoded, "\x00\\") {
		return "", false
	}
	for seg := range strings.SplitSeq(decoded, "/") {
		if seg == "." || seg == ".." {
			return "", false
		}
	}
	return decoded, true
}
