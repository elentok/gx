package oneoff

import "testing"

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"MyEpic":                "myepic",
		"fix the Login bug":     "fix-the-login-bug",
		"evolve-A_b.c-20261110": "evolve-a-b-c-20261110",
		"  --x--  ":             "x",
		"":                      SlugFallback,
		"../..":                 SlugFallback,
		"!!!":                   SlugFallback,
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
