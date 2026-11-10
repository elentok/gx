package devcheckout

import (
	"os"
	"path/filepath"
	"testing"
)

func checkout(t *testing.T, root string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "webui"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "webui", "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDevCheckout_Resolve(t *testing.T) {
	stamped := checkout(t, t.TempDir())
	override := checkout(t, t.TempDir())
	empty := t.TempDir()
	home := t.TempDir()
	checkout(t, filepath.Join(home, "code", "gx"))

	cases := []struct {
		name, override, stamp, want string
	}{
		{"stamped valid", "", stamped, stamped},
		{"override wins over stamped", override, stamped, override},
		{"override without package.json does not fall back", empty, stamped, ""},
		{"relative override does not fall back", "rel/path", stamped, ""},
		{"tilde expanded", "~/code/gx", stamped, filepath.Join(home, "code", "gx")},
		{"stamp without package.json", "", empty, ""},
		{"nothing set", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Resolve(c.override, c.stamp, home); got != c.want {
				t.Fatalf("Resolve(%q, %q) = %q, want %q", c.override, c.stamp, got, c.want)
			}
		})
	}
}
