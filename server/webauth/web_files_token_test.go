package webauth_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
)

func noFollowClient() *http.Client {
	c := *servertest.WebClient()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}

// tokenFixture is a signed-in login plus an HTML page and a sibling file.
type tokenFixture struct {
	l      *loginWeb
	cookie *http.Cookie
	dir    string
	page   string
}

func startTokenFixture(t *testing.T) tokenFixture {
	t.Helper()
	l := startLoginWeb(t)
	cookie, _ := l.signIn(t)
	dir := t.TempDir()
	for name, body := range map[string]string{"app/index.html": "<p>hi</p>\n", "app/a.css": "body{}\n", "other/x.txt": "x\n"} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return tokenFixture{l: l, cookie: cookie, dir: dir, page: filepath.Join(dir, "app", "index.html")}
}

// mint follows the stable link's redirect and returns the token URL path.
func (f tokenFixture) mint(t *testing.T) string {
	t.Helper()
	resp := f.noFollow(t, "/v1/files"+f.page, f.cookie)
	loc := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(loc, "/v1/files/t/") || !strings.HasSuffix(loc, f.page) {
		t.Fatalf("stable link: status %d location %q, want 302 to a token URL", resp.StatusCode, loc)
	}
	return loc
}

func (f tokenFixture) noFollow(t *testing.T, path string, cookie *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.l.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := noFollowClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestWebFilesToken_ServesItsFolderAndRedirectsOutside(t *testing.T) {
	t.Parallel()
	f := startTokenFixture(t)
	tok := f.mint(t)
	prefix := strings.TrimSuffix(tok, f.page)
	resp, body := f.l.get(t, tok, nil)
	if resp.StatusCode != http.StatusOK || body != "<p>hi</p>\n" || len(resp.Header.Values("Set-Cookie")) != 0 {
		t.Fatalf("token page: status %d body %q cookies %v, want 200 and no cookie", resp.StatusCode, body, resp.Header.Values("Set-Cookie"))
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("token 200: ACAO %q, want *", resp.Header.Get("Access-Control-Allow-Origin"))
	}
	if resp, body := f.l.get(t, prefix+filepath.Join(f.dir, "app", "a.css"), nil); resp.StatusCode != 200 || body != "body{}\n" {
		t.Errorf("sibling file: status %d body %q, want 200", resp.StatusCode, body)
	}
	outside := filepath.Join(f.dir, "other", "x.txt")
	r := f.noFollow(t, prefix+outside, nil)
	if r.StatusCode != http.StatusFound || r.Header.Get("Location") != "/v1/files"+outside {
		t.Errorf("outside folder: status %d location %q, want 302 to the stable link", r.StatusCode, r.Header.Get("Location"))
	}
	r = f.noFollow(t, prefix+f.dir+"/app/../other/x.txt", nil)
	if r.StatusCode != http.StatusFound || r.Header.Get("Location") != "/v1/files"+outside {
		t.Errorf("dotdot: status %d location %q, want 302 to the cleaned stable link", r.StatusCode, r.Header.Get("Location"))
	}
	req, _ := http.NewRequest(http.MethodGet, f.l.base+tok, nil)
	req.Header.Set("Range", "bytes=0-2")
	rr, err := noFollowClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rr.Body.Close()
	if rr.StatusCode != http.StatusPartialContent || rr.Header.Get("Content-Range") == "" || rr.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("range: status %d range %q, want 206 with Content-Range and ACAO", rr.StatusCode, rr.Header.Get("Content-Range"))
	}
	if raw, _ := f.l.get(t, "/v1/files"+f.dir+"/app/a.css", f.cookie); raw.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("raw route sent Access-Control-Allow-Origin")
	}
}

func TestWebFilesToken_ExpiresButTheStableLinkDoesNot(t *testing.T) {
	t.Parallel()
	f := startTokenFixture(t)
	tok := f.mint(t)
	f.l.advance(61 * time.Minute)
	r := f.noFollow(t, tok, nil)
	if r.StatusCode != http.StatusFound || r.Header.Get("Location") != "/v1/files"+f.page {
		t.Errorf("expired token: status %d location %q, want 302 to the stable link", r.StatusCode, r.Header.Get("Location"))
	}
	if r := f.noFollow(t, "/v1/files"+f.page, f.cookie); r.StatusCode != http.StatusFound {
		t.Errorf("stable link after an hour: status %d, want 302 with a fresh token", r.StatusCode)
	}
}

func TestWebFilesToken_DiesWithItsLoginAndARestart(t *testing.T) {
	t.Parallel()
	f := startTokenFixture(t)
	tok := f.mint(t)
	if resp, _ := f.l.post(t, "/v1/web/auth/sign-out", "{}", f.cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("sign-out status %d", resp.StatusCode)
	}
	if r := f.noFollow(t, tok, nil); r.StatusCode != http.StatusFound {
		t.Errorf("token after sign-out: status %d, want 302", r.StatusCode)
	}
	f2 := startTokenFixture(t)
	tok2 := f2.mint(t)
	if _, err := f2.l.h.Client.Web.SetPassword(context.Background(), "another-password-1"); err != nil {
		t.Fatal(err)
	}
	if r := f2.noFollow(t, tok2, nil); r.StatusCode != http.StatusFound {
		t.Errorf("token after web-password: status %d, want 302", r.StatusCode)
	}
}

func TestWebFilesToken_LoginLessSurface(t *testing.T) {
	t.Parallel()
	f := startTokenFixture(t)
	if r := f.noFollow(t, "/v1/files/t/bogus"+f.page, nil); r.StatusCode != http.StatusFound {
		t.Errorf("bad token: status %d, want 302", r.StatusCode)
	}
	for _, p := range []string{"/v1/files" + f.page, "/v1/snapshot", "/v1/files%2Ft%2F..%2Fsnapshot", "/v1/files/t/%2e%2e/%2e%2e/snapshot"} {
		r := f.noFollow(t, p, nil)
		if r.StatusCode != http.StatusUnauthorized && r.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d, want 401 or 404", p, r.StatusCode)
		}
		if strings.HasPrefix(p, "/v1/files") && r.Header.Get("Content-Security-Policy") == "" {
			t.Errorf("%s: no file headers", p)
		}
	}
	if r := f.noFollow(t, "/v1/files"+f.page+"?download=1", f.cookie); r.StatusCode != http.StatusOK {
		t.Errorf("download=1: status %d, want 200 and no redirect", r.StatusCode)
	}
}
