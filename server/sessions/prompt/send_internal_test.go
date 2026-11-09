package prompt

import (
	"errors"
	"slices"
	"testing"
)

type failingPaster struct {
	failAt int
	pasted []string
}

func (p *failingPaster) Paste(s string) error {
	if len(p.pasted) == p.failAt {
		return errors.New("paste failed")
	}
	p.pasted = append(p.pasted, s)
	return nil
}

func TestImageSetPaste_MarksAnImageOnlyAfterItsPaste(t *testing.T) {
	t.Parallel()
	var marked []string
	imgs := imageSet{
		ids:   []string{"a", "b", "c"},
		paths: []string{"/a", "/b", "/c"},
		sent:  func(id string) { marked = append(marked, id) },
	}
	c := &failingPaster{failAt: 1}
	if err := imgs.paste(c); err == nil {
		t.Fatal("paste = nil, want the second paste's error")
	}
	if want := []string{"a"}; !slices.Equal(marked, want) {
		t.Errorf("marked = %v, want %v", marked, want)
	}
}

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
