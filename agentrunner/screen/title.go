package screen

import "bytes"

var oscTitle = []byte("\x1b]0;")

// Title returns the last complete OSC 0 window title in raw, and false when
// raw holds none. It scans raw bytes because x/vt's title callback cuts a
// title at any 0x9C byte, which is inside glyphs like ✳ (E2 9C B3).
func Title(raw []byte) (string, bool) {
	for end := len(raw); ; {
		i := bytes.LastIndex(raw[:end], oscTitle)
		if i < 0 {
			return "", false
		}
		body := raw[i+len(oscTitle):]
		if n := bytes.IndexAny(body, "\x07\x1b"); n >= 0 && (body[n] == '\x07' || bytes.HasPrefix(body[n:], []byte("\x1b\\"))) {
			return string(body[:n]), true
		}
		end = i
	}
}
