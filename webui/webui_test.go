package webui

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestPick(t *testing.T) {
	placeholderFS := fstest.MapFS{"placeholder/index.html": {Data: []byte("placeholder")}}

	t.Run("prefers dist when it has an index", func(t *testing.T) {
		distFS := fstest.MapFS{"dist/index.html": {Data: []byte("built")}}
		got, err := fs.ReadFile(pick(distFS, placeholderFS), "index.html")
		if err != nil || string(got) != "built" {
			t.Fatalf("got %q, %v; want built", got, err)
		}
	})

	t.Run("falls back to the placeholder", func(t *testing.T) {
		distFS := fstest.MapFS{"dist/.gitkeep": {}}
		got, err := fs.ReadFile(pick(distFS, placeholderFS), "index.html")
		if err != nil || string(got) != "placeholder" {
			t.Fatalf("got %q, %v; want placeholder", got, err)
		}
	})
}
