package screen

// stFilter guards control strings (OSC, DCS, SOS, PM, APC) against the
// emulator's parser, which takes any 0x9C byte in them as the 8-bit ST that
// ends the string, even mid UTF-8 rune. The rest of the string then prints as
// text at the cursor: claude's "✳ <name>" window title (✳ is E2 9C B3) ends up
// on its input line. stFilter swaps such a 0x9C for another continuation byte,
// keeping the string's length, and keeps a lone 8-bit ST as it is. Its state
// carries across writes, as a string can span reads.
type stFilter struct {
	state stState
	osc   bool // the string is an OSC, which BEL also ends
	need  int  // continuation bytes still due for the current rune
}

type stState uint8

const (
	stGround stState = iota
	stEscape
	stString
)

// stSubstitute stands in for a 0x9C continuation byte.
const stSubstitute = 0xBF

// filter rewrites p in place.
func (f *stFilter) filter(p []byte) {
	for i, b := range p {
		switch f.state {
		case stGround:
			if b == 0x1B {
				f.state = stEscape
			}
		case stEscape:
			switch b {
			case 0x1B:
			case ']', 'P', 'X', '^', '_':
				f.state, f.osc, f.need = stString, b == ']', 0
			default:
				f.state = stGround
			}
		case stString:
			p[i] = f.stringByte(b)
		}
	}
}

func (f *stFilter) stringByte(b byte) byte {
	switch {
	case b == 0x1B:
		f.state = stEscape
	case b == 0x18 || b == 0x1A || (b == 0x07 && f.osc):
		f.state = stGround
	case b >= 0x80 && b <= 0xBF && f.need > 0:
		f.need--
		if b == 0x9C {
			return stSubstitute
		}
	case b == 0x9C:
		f.state = stGround
	case b >= 0xC0 && b <= 0xDF:
		f.need = 1
	case b >= 0xE0 && b <= 0xEF:
		f.need = 2
	case b >= 0xF0 && b <= 0xF7:
		f.need = 3
	default:
		f.need = 0
	}
	return b
}
