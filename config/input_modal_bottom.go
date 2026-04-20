package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// InputModalBottomKind identifies the variant of InputModalBottom.
type InputModalBottomKind int

const (
	InputModalBottomKindPercent InputModalBottomKind = iota
	InputModalBottomKindLines
	InputModalBottomKindCenter
)

// InputModalBottom controls how far from the bottom of the screen the
// text-input overlay is placed.  It can be:
//   - a percentage of screen height (e.g. "5%")
//   - a fixed number of lines (e.g. 10)
//   - the string "center" to center vertically
type InputModalBottom struct {
	Kind    InputModalBottomKind
	Lines   int
	Percent float64
}

// DefaultInputModalBottom returns the default value (5% from the bottom).
func DefaultInputModalBottom() InputModalBottom {
	return InputModalBottom{Kind: InputModalBottomKindLines, Lines: 3}
}

// ResolveY computes the top-left y coordinate for the overlay given the
// screen height and the overlay's rendered height.
func (b InputModalBottom) ResolveY(screenH, fgH int) int {
	var y int
	switch b.Kind {
	case InputModalBottomKindLines:
		y = screenH - fgH - b.Lines
	case InputModalBottomKindPercent:
		pad := int(float64(screenH) * b.Percent / 100)
		y = screenH - fgH - pad
	case InputModalBottomKindCenter:
		y = (screenH - fgH) / 2
	}
	if y < 0 {
		y = 0
	}
	return y
}

func (b InputModalBottom) MarshalJSON() ([]byte, error) {
	switch b.Kind {
	case InputModalBottomKindLines:
		return json.Marshal(b.Lines)
	case InputModalBottomKindPercent:
		return json.Marshal(strconv.FormatFloat(b.Percent, 'f', -1, 64) + "%")
	case InputModalBottomKindCenter:
		return json.Marshal("center")
	default:
		return nil, fmt.Errorf("unknown InputModalBottomKind %d", b.Kind)
	}
}

func (b *InputModalBottom) UnmarshalJSON(data []byte) error {
	// Try numeric first.
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		b.Kind = InputModalBottomKindLines
		b.Lines = n
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		*b = DefaultInputModalBottom()
		return nil
	}

	if s == "center" {
		b.Kind = InputModalBottomKindCenter
		return nil
	}

	if strings.HasSuffix(s, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil || pct < 0 {
			*b = DefaultInputModalBottom()
			return nil
		}
		b.Kind = InputModalBottomKindPercent
		b.Percent = pct
		return nil
	}

	*b = DefaultInputModalBottom()
	return nil
}
