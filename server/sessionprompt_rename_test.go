package server

import "testing"

func TestRenameCommandArg(t *testing.T) {
	cases := []struct {
		in   string
		name string
		ok   bool
	}{
		{"/rename blabla", "blabla", true},
		{"  /name  my agent ", "my agent", true},
		{"/rename", "", false},
		{"/rename   ", "", false},
		{"/renamed x", "", false},
		{"hello /rename x", "", false},
		{"/rename a\nb", "", false},
	}
	for _, c := range cases {
		name, ok := renameCommandArg(c.in)
		if name != c.name && ok || ok != c.ok {
			t.Errorf("renameCommandArg(%q) = %q, %v; want %q, %v", c.in, name, ok, c.name, c.ok)
		}
	}
}
