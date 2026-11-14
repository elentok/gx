package files

import "testing"

func TestViewerURL_GoldenExamples(t *testing.T) {
	const base = "https://localhost:8443"
	tests := []struct{ name, path, viewer, raw string }{
		{"plain", "/home/me/a.md", base + "/#/files/home/me/a.md", base + "/v1/files/home/me/a.md"},
		{"space and non-ascii", "/home/me/my notes/café.md", base + "/#/files/home/me/my%20notes/caf%C3%A9.md", base + "/v1/files/home/me/my%20notes/caf%C3%A9.md"},
		{"hash and question mark", "/home/me/a#b?c.md", base + "/#/files/home/me/a%23b%3Fc.md", base + "/v1/files/home/me/a%23b%3Fc.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ViewerURL(base, tt.path); got != tt.viewer {
				t.Errorf("ViewerURL = %q, want %q", got, tt.viewer)
			}
			if got := RawURL(base, tt.path); got != tt.raw {
				t.Errorf("RawURL = %q, want %q", got, tt.raw)
			}
		})
	}
}

func TestHasRawLink(t *testing.T) {
	for path, want := range map[string]bool{"/t/x.md": false, "/tmp/x.md": true, "/a/t/x.md": true} {
		if got := HasRawLink(path); got != want {
			t.Errorf("HasRawLink(%q) = %v, want %v", path, got, want)
		}
	}
}
