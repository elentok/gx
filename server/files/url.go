package files

import (
	"net/url"
	"strings"
)

// ViewerURL is the link a person opens to see a hosted file: base + "/#/files/"
// + the absolute path with each segment percent-encoded. The web client builds
// the same form; the golden examples in url_test.go are mirrored there.
func ViewerURL(base, path string) string {
	return base + "/#/files" + EscapePath(path)
}

// RawURL is the raw link of a hosted file: base + "/v1/files/" + the path.
func RawURL(base, path string) string {
	return base + "/v1/files" + EscapePath(path)
}

// EscapePath percent-encodes each segment of an absolute path, so "#", "?" and
// spaces in a name cannot end the path early.
func EscapePath(path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// HasRawLink reports whether path can be reached by a raw link. A top-level
// folder named "t" collides with the token route, so nothing under it can.
func HasRawLink(path string) bool {
	return !strings.HasPrefix(path, "/t/")
}
