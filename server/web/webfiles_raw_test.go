package web_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rawGet sends GET base+target without cleaning or following anything. A nil
// cookie sends none.
func rawGet(t *testing.T, base, target string, cookie *http.Cookie) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWebFiles_RawServesHostedFilesBehindTheLogin(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	writeFile(t, filepath.Join(f.repo, "keys", "id_rsa.pub"), "public\n")
	plan := f.repo + "/docs/plan.md"
	resp, body := rawGet(t, f.base, "/v1/files"+plan, f.cookie)
	if resp.StatusCode != http.StatusOK || body != "# plan\n" {
		t.Fatalf("status %d body %q, want 200 and the bytes", resp.StatusCode, body)
	}
	if resp, _ := rawGet(t, f.base, "/v1/files"+plan, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no cookie: status %d, want 401", resp.StatusCode)
	}
	if resp, body := rawGet(t, f.base, "/v1/files"+f.repo+"/keys/id_rsa.pub", f.cookie); resp.StatusCode != 200 || body != "public\n" {
		t.Errorf("id_rsa.pub: status %d, want 200", resp.StatusCode)
	}
	if resp, body := rawGet(t, f.base, "/files"+plan, f.cookie); strings.Contains(body, "# plan") {
		t.Errorf("/files/<path> served the file (status %d)", resp.StatusCode)
	}
}

func TestWebFiles_RawAnswersOneNotFound(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	link := filepath.Join(f.repo, "link.txt")
	if err := os.Symlink("/etc/hosts", link); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(f.h.StateDir, "sessions", "s.txt"), "x\n")
	writeFile(t, filepath.Join(f.repo, "keys", "id_rsa.pub"), "public\n")
	cases := map[string]string{
		"missing":      f.repo + "/nope.md",
		"denied .env":  f.repo + "/.env",
		"denied key":   f.repo + "/id_rsa",
		"symlink out":  link,
		"no root":      "/etc/hosts",
		"gx sessions":  filepath.Join(f.h.StateDir, "sessions", "s.txt"),
		"dotdot":       f.repo + "/docs/../sibling.md",
		"token prefix": "/t/abc",
	}
	var want string
	for name, path := range cases {
		resp, body := rawGet(t, f.base, "/v1/files"+path, f.cookie)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", name, resp.StatusCode)
		}
		if want == "" {
			want = body
		} else if body != want {
			t.Errorf("%s: body %q differs from %q", name, body, want)
		}
	}
}

func TestWebFiles_RawTempFileNeedsTheGxOwner(t *testing.T) {
	t.Parallel()
	tmp := filepath.Join(t.TempDir(), "t.txt")
	writeFile(t, tmp, "temp\n")
	own := startResolve(t, nil)
	if resp, _ := rawGet(t, own.base, "/v1/files"+tmp, own.cookie); resp.StatusCode != 200 {
		t.Errorf("own temp file: status %d, want 200", resp.StatusCode)
	}
	foreign := uint32(os.Getuid()) + 1
	other := startResolve(t, &foreign)
	if resp, _ := rawGet(t, other.base, "/v1/files"+tmp, other.cookie); resp.StatusCode != 404 {
		t.Errorf("foreign-owned temp file: status %d, want 404", resp.StatusCode)
	}
}

func TestWebFiles_RawRefusesNonCanonicalPaths(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	p := f.repo + "/docs/plan.md"
	targets := map[string]string{
		"%2e%2e":        "/v1/files" + f.repo + "/docs/%2e%2e/sibling.md",
		"%2F":           "/v1/files" + strings.ReplaceAll(p, "/docs/", "%2Fdocs%2F"),
		"%5C":           "/v1/files" + f.repo + "/docs%5Cplan.md",
		"double slash":  "/v1/files" + f.repo + "//docs/plan.md",
		"backslash":     "/v1/files" + f.repo + `/docs\plan.md`,
		"NUL":           "/v1/files" + p + "%00",
		"dot segment":   "/v1/files" + f.repo + "/docs/./plan.md",
		"encoded files": "/v1/files%2Ft%2F..%2Fsnapshot",
	}
	for name, target := range targets {
		resp, _ := rawGet(t, f.base, target, f.cookie)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", name, resp.StatusCode)
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no file headers", name)
		}
	}
}

func TestWebFiles_RawTicketMockServedDataSessionsNot(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	mock := filepath.Join(f.store, "proj", "epic-a", "mocks", "m.html")
	writeFile(t, mock, "<p>mock</p>")
	if resp, body := getHTML(t, f, mock); resp.StatusCode != 200 || body != "<p>mock</p>" {
		t.Errorf("mock: status %d body %q, want 200", resp.StatusCode, body)
	}
}
