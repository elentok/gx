package web_test

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// getHTML asks for an HTML file on the raw route and follows the one redirect
// to the token URL, the way the viewer's frame does.
func getHTML(t *testing.T, f resolveFixture, path string) (*http.Response, string) {
	t.Helper()
	resp, _ := rawGet(t, f.base, "/v1/files"+path, f.cookie)
	loc := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(loc, "/v1/files/t/") {
		t.Fatalf("raw html: status %d location %q, want 302 to a token URL", resp.StatusCode, loc)
	}
	return rawGet(t, f.base, loc, nil)
}

func TestWebFilesToken_HTMLRedirectsToTokenURLThatServesIt(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	page := filepath.Join(f.repo, "site", "index.html")
	writeFile(t, page, "<p>site</p>")
	writeFile(t, filepath.Join(f.repo, "site", "app.css"), "a{}")
	resp, body := getHTML(t, f, page)
	if resp.StatusCode != http.StatusOK || body != "<p>site</p>" || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("token page: status %d body %q", resp.StatusCode, body)
	}
	if resp, _ := rawGet(t, f.base, "/v1/files"+page+"?download=1", f.cookie); resp.StatusCode != http.StatusOK {
		t.Errorf("download=1: status %d, want 200 without a redirect", resp.StatusCode)
	}
	if resp, _ := rawGet(t, f.base, "/v1/files"+filepath.Join(f.repo, "site", "app.css"), f.cookie); resp.StatusCode != http.StatusOK {
		t.Errorf("css: status %d, want 200 served directly", resp.StatusCode)
	}
}

func TestWebFilesToken_SameNotFoundAsRaw(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	page := filepath.Join(f.repo, "site", "index.html")
	writeFile(t, page, "<p>site</p>")
	writeFile(t, filepath.Join(f.repo, "site", ".env"), "SECRET=1")
	resp, _ := rawGet(t, f.base, "/v1/files"+page, f.cookie)
	prefix := strings.TrimSuffix(resp.Header.Get("Location"), page)
	for _, name := range []string{"/site/.env", "/site/missing.txt"} {
		if resp, _ := rawGet(t, f.base, prefix+f.repo+name, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", name, resp.StatusCode)
		}
	}
}
