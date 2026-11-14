package files

import "testing"

func TestPathForm(t *testing.T) {
	cases := []struct {
		name, in, want string
		ok             bool
	}{
		{"canonical", "/home/a/notes.md", "/home/a/notes.md", true},
		{"encoded space", "/home/a/my%20notes.md", "/home/a/my notes.md", true},
		{"encoded dotdot", "/home/a/%2e%2e/b", "", false},
		{"encoded dotdot upper", "/home/a/%2E%2E/b", "", false},
		{"encoded slash", "/home/a%2Fb", "", false},
		{"encoded slash lower", "/home/a%2fb", "", false},
		{"encoded backslash", "/home/a%5Cb", "", false},
		{"double slash", "/home//a", "", false},
		{"backslash", `/home/a\b`, "", false},
		{"NUL encoded", "/home/a%00b", "", false},
		{"NUL raw", "/home/a\x00b", "", false},
		{"dot segment", "/home/./a", "", false},
		{"dotdot segment", "/home/../a", "", false},
		{"encoded dot", "/home/%2e/a", "", false},
		{"relative", "home/a", "", false},
		{"bad escape", "/home/%zz", "", false},
		{"dotfile is fine", "/home/a/.notes", "/home/a/.notes", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := PathForm(c.in)
			if ok != c.ok || got != c.want {
				t.Fatalf("PathForm(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}
