package tickets

import (
	"regexp"
	"strings"
)

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// SlugFallback is what Slug returns when nothing of s survives.
const SlugFallback = "one-off"

// Slug turns s into a name of [a-z0-9-]: lowercased, every other run of
// characters one dash, no leading or trailing dash. Nothing left gives
// SlugFallback. Lowercase first: an uppercase letter is not [a-z0-9] and would
// otherwise be eaten.
func Slug(s string) string {
	slug := strings.Trim(nonSlugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if slug == "" {
		return SlugFallback
	}
	return slug
}
