package web_test

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// rawReq sends GET base+target with extra request headers.
func rawReq(t *testing.T, f resolveFixture, target string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.base+target, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(f.cookie)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

var fileHeaderNames = []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy", "Cache-Control"}

func TestWebFilesRange_PartialContent(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	p := filepath.Join(f.repo, "notes.txt")
	writeFile(t, p, "0123456789abcdef")
	full, _ := rawReq(t, f, "/v1/files"+p, nil)

	resp, body := rawReq(t, f, "/v1/files"+p, map[string]string{"Range": "bytes=0-9"})
	if resp.StatusCode != http.StatusPartialContent || body != "0123456789" {
		t.Fatalf("status %d body %q, want 206 and 10 bytes", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 0-9/16" {
		t.Errorf("Content-Range = %q", got)
	}
	for _, n := range fileHeaderNames {
		if resp.Header.Get(n) == "" || resp.Header.Get(n) != full.Header.Get(n) {
			t.Errorf("%s: 206 %q, 200 %q", n, resp.Header.Get(n), full.Header.Get(n))
		}
	}
	if resp.Header.Get("Content-Type") != full.Header.Get("Content-Type") {
		t.Errorf("Content-Type 206 %q, 200 %q", resp.Header.Get("Content-Type"), full.Header.Get("Content-Type"))
	}
}

func TestWebFilesRange_Unsatisfiable(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	p := filepath.Join(f.repo, "notes.txt")
	writeFile(t, p, "short")
	resp, _ := rawReq(t, f, "/v1/files"+p, map[string]string{"Range": "bytes=100-200"})
	if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status %d, want 416", resp.StatusCode)
	}
	for _, n := range fileHeaderNames {
		if resp.Header.Get(n) == "" {
			t.Errorf("416 lacks %s", n)
		}
	}
}

func TestWebFilesRange_DownloadIsAttachmentNeverRedirect(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	for _, name := range []string{"page.html", "my notes.txt"} {
		p := filepath.Join(f.repo, name)
		writeFile(t, p, "<p>x</p>")
		resp, body := rawReq(t, f, "/v1/files"+strings.ReplaceAll(p, " ", "%20")+"?download=1", nil)
		if resp.StatusCode != http.StatusOK || body != "<p>x</p>" {
			t.Fatalf("%s: status %d body %q, want 200", name, resp.StatusCode, body)
		}
		cd := resp.Header.Get("Content-Disposition")
		if !strings.HasPrefix(cd, "attachment; filename=") || !strings.Contains(cd, strings.Split(name, " ")[0]) {
			t.Errorf("%s: Content-Disposition = %q", name, cd)
		}
	}
}

func TestWebFilesRange_PDF(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	p := filepath.Join(f.repo, "doc.pdf")
	writeFile(t, p, "%PDF-1.4\n")
	resp, _ := rawReq(t, f, "/v1/files"+p, nil)
	if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox allow-scripts") {
		t.Errorf("CSP = %q", csp)
	}
	if resp.Header.Get("Content-Disposition") != "" {
		t.Errorf("inline PDF has Content-Disposition %q", resp.Header.Get("Content-Disposition"))
	}
	resp, _ = rawReq(t, f, "/v1/files"+p+"?download=1", nil)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment; filename=") {
		t.Errorf("download: status %d, CD %q", resp.StatusCode, resp.Header.Get("Content-Disposition"))
	}
}
