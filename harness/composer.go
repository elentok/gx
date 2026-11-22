package harness

import "bytes"

// ComposerScanner reads an agent's raw output, fed in chunks as it is read,
// and reports whether the composer has been drawn. It is a separate read from
// the screen: Claude draws its composer marker once, in the full draw, and
// later repaints are diffs.
type ComposerScanner interface {
	// Feed takes the next chunk and reports whether the composer was seen in
	// this or an earlier chunk.
	Feed(chunk []byte) bool
}

// ComposerScanning is implemented by harnesses that can tell from the raw
// output that the composer is up.
type ComposerScanning interface {
	NewComposerScanner() ComposerScanner
}

// claudeComposerMarker is the pointer glyph followed by U+00A0. A startup menu
// (trust dialog, theme menu) draws the pointer followed by a cursor move, so
// the no-break space is what tells the composer apart.
var claudeComposerMarker = []byte("❯ ")

type claudeComposerScanner struct {
	tail []byte
	seen bool
}

func (s *claudeComposerScanner) Feed(chunk []byte) bool {
	if s.seen {
		return true
	}
	data := append(s.tail, chunk...)
	if bytes.Contains(data, claudeComposerMarker) {
		s.seen, s.tail = true, nil
		return true
	}
	keep := min(len(claudeComposerMarker)-1, len(data))
	s.tail = bytes.Clone(data[len(data)-keep:])
	return false
}

func (claudeHarness) NewComposerScanner() ComposerScanner { return &claudeComposerScanner{} }

// codexComposerScanner takes Codex's OSC 7501 idle report as the composer
// signal: the `›` row also shows in its loading frame, which is too early.
type codexComposerScanner struct {
	parser ProgramStatusParser
	seen   bool
}

func (s *codexComposerScanner) Feed(chunk []byte) bool {
	_, reports := s.parser.Feed(chunk)
	for _, r := range reports {
		if r.State == "idle" && r.App == "codex" {
			s.seen = true
		}
	}
	return s.seen
}

func (codexHarness) NewComposerScanner() ComposerScanner { return &codexComposerScanner{} }
