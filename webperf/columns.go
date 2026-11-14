package webperf

import "math"

// StrCol is a dictionary-coded string column: row i is Dict[Codes[i]]. Rows
// with no value use the empty string.
type StrCol struct {
	Dict  []string
	Codes []uint32
}

// At returns the value of row i.
func (c *StrCol) At(i int) string { return c.Dict[c.Codes[i]] }

type strBuilder struct {
	col StrCol
	idx map[string]uint32
}

func newStrBuilder(n int) *strBuilder {
	return &strBuilder{col: StrCol{Codes: make([]uint32, 0, n)}, idx: map[string]uint32{}}
}

func (b *strBuilder) add(v string) {
	code, ok := b.idx[v]
	if !ok {
		code = uint32(len(b.col.Dict))
		b.col.Dict = append(b.col.Dict, v)
		b.idx[v] = code
	}
	b.col.Codes = append(b.col.Codes, code)
}

// Day holds one day's samples as columns. Numeric columns use NaN for an
// absent value. All columns have N rows, in file order.
type Day struct {
	Day string
	N   int

	Metric, Route, BuildID, WebVersion, Env, NavigationType, Tab, Session StrCol
	Action, WorstChunk                                                    StrCol

	Value                                     []float64
	TTFB, APILatency, JSBytes, CSSBytes       []float64
	InputDelay, Processing, PresentationDelay []float64
}

func opt(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}

func buildDay(day string, samples []Sample) *Day {
	n := len(samples)
	d := &Day{Day: day, N: n}
	strs := []struct {
		dst *StrCol
		get func(*Sample) string
	}{
		{&d.Metric, func(s *Sample) string { return s.Metric }},
		{&d.Route, func(s *Sample) string { return s.Route }},
		{&d.BuildID, func(s *Sample) string { return s.BuildID }},
		{&d.WebVersion, func(s *Sample) string { return s.WebVersion }},
		{&d.Env, func(s *Sample) string { return s.Env }},
		{&d.NavigationType, func(s *Sample) string { return s.NavigationType }},
		{&d.Tab, func(s *Sample) string { return s.Tab }},
		{&d.Session, func(s *Sample) string { return s.Session }},
		{&d.Action, func(s *Sample) string { return s.Action }},
		{&d.WorstChunk, func(s *Sample) string { return s.WorstChunk }},
	}
	for _, c := range strs {
		b := newStrBuilder(n)
		for i := range samples {
			b.add(c.get(&samples[i]))
		}
		*c.dst = b.col
	}
	nums := []struct {
		dst *[]float64
		get func(*Sample) float64
	}{
		{&d.Value, func(s *Sample) float64 { return s.Value }},
		{&d.TTFB, func(s *Sample) float64 { return opt(s.TTFB) }},
		{&d.APILatency, func(s *Sample) float64 { return opt(s.APILatency) }},
		{&d.JSBytes, func(s *Sample) float64 { return opt(s.JSBytes) }},
		{&d.CSSBytes, func(s *Sample) float64 { return opt(s.CSSBytes) }},
		{&d.InputDelay, func(s *Sample) float64 { return opt(s.InputDelay) }},
		{&d.Processing, func(s *Sample) float64 { return opt(s.Processing) }},
		{&d.PresentationDelay, func(s *Sample) float64 { return opt(s.PresentationDelay) }},
	}
	for _, c := range nums {
		col := make([]float64, n)
		for i := range samples {
			col[i] = c.get(&samples[i])
		}
		*c.dst = col
	}
	return d
}
