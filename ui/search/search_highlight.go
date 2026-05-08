package search

import (
	"strings"

	"github.com/elentok/gx/ui"
)

func Highlight(text, query string, isActive bool) string {
	if strings.TrimSpace(query) == "" {
		return text
	}
	lower := strings.ToLower(text)
	lq := strings.ToLower(query)
	idx := strings.Index(lower, lq)
	if idx < 0 {
		return text
	}
	end := min(idx+len(query), len(text))

	style := ui.StyleSearchResult
	if isActive {
		style = ui.StyleActiveSearchResult
	}
	return text[:idx] + style.Render(text[idx:end]) + text[end:]
}
