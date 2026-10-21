package cmd

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func startWebServer(t *testing.T) *servertest.Harness {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	return servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
}

func webGet(t *testing.T, url string, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestWebToken_PrintsTheCurrentTokenAndRotateReplacesIt(t *testing.T) {
	h := startWebServer(t)
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	old, err := server.ReadWebToken(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	signIn := webGet(t, hs.WebURL+"/?gx_token="+old)
	if len(signIn.Cookies()) != 1 {
		t.Fatalf("sign-in set %d cookies, want 1", len(signIn.Cookies()))
	}
	cookie := signIn.Cookies()[0]
	if got := webGet(t, hs.WebURL+"/", cookie).StatusCode; got != http.StatusOK {
		t.Fatalf("cookie before rotate = %d, want 200", got)
	}

	var out bytes.Buffer
	if err := runWebToken(h.StateDir, false, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != old+"\n" {
		t.Errorf("web-token = %q, want the current token", out.String())
	}

	out.Reset()
	if err := runWebToken(h.StateDir, true, &out); err != nil {
		t.Fatal(err)
	}
	fresh, _ := server.ReadWebToken(h.StateDir)
	if fresh == old || out.String() != fresh+"\n" {
		t.Errorf("rotate printed %q, file holds %q, old was %q", out.String(), fresh, old)
	}
	if got := webGet(t, hs.WebURL+"/", cookie).StatusCode; got != http.StatusUnauthorized {
		t.Errorf("old cookie after rotate = %d, want 401", got)
	}
}

func TestWebURL_QRFlagPrintsAQRForTheTokenedURL(t *testing.T) {
	h := startWebServer(t)
	token, err := server.ReadWebToken(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantURL := hs.WebURL + "/?gx_token=" + token

	var plain bytes.Buffer
	if err := runWebURL(context.Background(), h.Client, h.StateDir, false, &plain); err != nil {
		t.Fatal(err)
	}
	if plain.String() != wantURL+"\n" {
		t.Errorf("web-url = %q, want %q", plain.String(), wantURL+"\n")
	}

	var qr bytes.Buffer
	if err := runWebURL(context.Background(), h.Client, h.StateDir, true, &qr); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(qr.String(), wantURL+"\n") {
		t.Errorf("web-url --qr = %q, want it to end with the URL", qr.String())
	}
	lines := strings.Split(strings.TrimSuffix(qr.String(), wantURL+"\n"), "\n")
	// A version-4 code is 33 modules tall plus a quiet zone; two modules fit in one text row.
	if len(lines) < 17 || !strings.ContainsAny(qr.String(), "▀▄█") {
		t.Errorf("web-url --qr printed %d rows without block characters:\n%s", len(lines), qr.String())
	}
}

func TestWebURL_WebListenerOffIsAnError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	h := servertest.Start(t)
	err := runWebURL(context.Background(), h.Client, h.StateDir, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "web UI is off") {
		t.Errorf("err = %v, want a web UI is off error", err)
	}
}
