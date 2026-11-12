package mockcheck

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePNG(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidate(t *testing.T) {
	good := writePNG(t, "a.png")
	empty := filepath.Join(t.TempDir(), "empty.png")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		text string
		want string // substring of the single problem, "" for none
	}{
		{"pass with viewable pngs", "down: pass " + good + " " + good, ""},
		{"fail and unchecked need no pngs", "a: fail gap differs\nb: unchecked empty read\n", ""},
		{"pass without paths", "down: pass", "needs the mock and built PNG paths"},
		{"pass with one path", "down: pass " + good, "needs the mock and built PNG paths"},
		{"pass with empty png", "down: pass " + good + " " + empty, "unviewable"},
		{"pass with missing png", "down: pass " + good + " /nope.png", "unviewable"},
		{"unknown verdict", "down: matches", "unknown verdict"},
		{"no state", "pass", "want `<state>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Validate(tc.text)
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no problems, got %v", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("want one problem containing %q, got %v", tc.want, got)
			}
		})
	}
}
