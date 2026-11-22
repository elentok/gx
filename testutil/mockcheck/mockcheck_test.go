package mockcheck

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePNGSize(t *testing.T, name string, side int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7 % 251)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePNG(t *testing.T, name string) string { return writePNGSize(t, name, 32) }

func TestValidateReportsEachImage(t *testing.T) {
	a, b := writePNG(t, "a.png"), writePNG(t, "b.png")
	problems, viewed := Validate("down: pass " + a + " " + b + " | [s] [p] | [s] [p]")
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if len(viewed) != 2 || !strings.Contains(viewed[0], a) || !strings.Contains(viewed[1], b) {
		t.Fatalf("want one line per image, got %v", viewed)
	}
}

func TestValidate(t *testing.T) {
	good := writePNG(t, "a.png")
	tiny := writePNGSize(t, "tiny.png", 1)
	sel := " | [s] [p] | [s] [p]"
	empty := filepath.Join(t.TempDir(), "empty.png")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		text string
		want string // substring of the single problem, "" for none
	}{
		{"pass with viewable pngs", "down: pass " + good + " " + good + sel, ""},
		{"fail and unchecked need no pngs", "a: fail gap differs\nb: unchecked empty read\n", ""},
		{"pass without paths", "down: pass", "needs the mock and built PNG paths"},
		{"pass with one path", "down: pass " + good + sel, "needs the mock and built PNG paths"},
		{"pass with empty png", "down: pass " + good + " " + empty + sel, "unviewable"},
		{"pass with missing png", "down: pass " + good + " /nope.png" + sel, "unviewable"},
		{"pass with tiny png", "down: pass " + good + " " + tiny + sel, "blank capture"},
		{"pass without selectors", "down: pass " + good + " " + good, "needs `| <mock selector>"},
		{"pass with different selectors", "down: pass " + good + " " + good + " | [a] [p] | [b] [p]", "different selectors"},
		{"unknown verdict", "down: matches", "unknown verdict"},
		{"no state", "pass", "want `<state>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := Validate(tc.text)
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
