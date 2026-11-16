package ticketspec

import (
	"slices"
	"testing"
)

func TestFenceLines(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []fenceLine
	}{
		{"plain fence", "a\n```\nb\n```\nc", []fenceLine{
			{Text: "a"}, {Text: "b", InFence: true}, {Text: "c"},
		}},
		{"text fence", "```text\nb\n```\nc", []fenceLine{
			{Text: "b", InFence: true, Info: "text"}, {Text: "c"},
		}},
		{"unclosed fence", "a\n```go\nb\nc", []fenceLine{
			{Text: "a"}, {Text: "b", InFence: true, Info: "go"}, {Text: "c", InFence: true, Info: "go"},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := slices.Collect(fenceLines(c.text))
			if !slices.Equal(got, c.want) {
				t.Errorf("fenceLines(%q) = %+v, want %+v", c.text, got, c.want)
			}
		})
	}
}
