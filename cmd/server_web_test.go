package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func startWebServer(t *testing.T) *servertest.Harness {
	t.Helper()
	return servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
}

func TestServerStatus_NotRunningFromATempEnv(t *testing.T) {
	var stdout bytes.Buffer
	d := deps{stdout: &stdout, stderr: &bytes.Buffer{}, env: tempEnv(t)}
	if err := execute([]string{"server", "status"}, d); err != nil {
		t.Fatalf("server status: %v", err)
	}
	if !strings.Contains(stdout.String(), "not running") {
		t.Errorf("status = %q, want it to say not running", stdout.String())
	}
}

func TestServerWebURL_NoHomeInEnvIsAnError(t *testing.T) {
	d := deps{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	err := execute([]string{"server", "web-url"}, d)
	if err == nil || !strings.Contains(err.Error(), "no HOME in env") {
		t.Errorf("err = %v, want it to mention no HOME in env", err)
	}
}

func webGet(t *testing.T, url string, cookies ...*http.Cookie) *http.Response {
	t.Helper()
	client := &http.Client{Transport: servertest.WebTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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
	if err := runWebURL(context.Background(), h.Client, h.StateDir, false, false, &plain); err != nil {
		t.Fatal(err)
	}
	if plain.String() != wantURL+"\n" {
		t.Errorf("web-url = %q, want %q", plain.String(), wantURL+"\n")
	}

	var qr bytes.Buffer
	if err := runWebURL(context.Background(), h.Client, h.StateDir, true, false, &qr); err != nil {
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

func TestWebURL_UsesAnAllowedHostSetBeforeTheRootIsMade(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.WebAllowedHosts = []string{"gx.example.com"}
	})
	token, err := server.ReadWebToken(h.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, qr := range []bool{false, true} {
		var out bytes.Buffer
		if err := runWebURL(context.Background(), h.Client, h.StateDir, qr, false, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "https://gx.example.com:") || !strings.Contains(out.String(), "/?gx_token="+token) {
			t.Errorf("web-url (qr=%v) = %q, want the allowed host", qr, out.String())
		}
	}
}

func TestWebURL_AllInterfacesBindNeverPrintsTheUnspecifiedAddress(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ifaces []net.IP
		want   string
	}{
		{"private interface", []net.IP{net.ParseIP("192.168.1.7")}, "https://192.168.1.7:"},
		{"no private interface", []net.IP{net.ParseIP("8.8.8.8")}, "https://localhost:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
				c.WebAddr = "0.0.0.0:0"
				c.WebInterfaceIPs = func() []net.IP { return tc.ifaces }
			})
			hs, err := h.Client.Handshake(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(hs.WebURL, tc.want) || strings.Contains(hs.WebURL, "0.0.0.0") {
				t.Errorf("web url = %q, want prefix %q and no 0.0.0.0", hs.WebURL, tc.want)
			}
		})
	}
}

func TestWebURL_WebListenerOffIsAnError(t *testing.T) {
	h := servertest.Start(t)
	err := runWebURL(context.Background(), h.Client, h.StateDir, false, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "web UI is off") {
		t.Errorf("err = %v, want a web UI is off error", err)
	}
}

func TestWebURL_NoServerIsServerNotRunning(t *testing.T) {
	// A short path: t.TempDir() can exceed the unix socket path limit.
	dir, err := os.MkdirTemp("/tmp", "gxn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	cl := apiclient.New(filepath.Join(dir, "none.sock"))
	err = runWebURL(context.Background(), cl, t.TempDir(), false, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), server.ReasonServerNotRunning) {
		t.Errorf("err = %v, want %s", err, server.ReasonServerNotRunning)
	}
}

func TestWebURL_JSONCarriesBaseURLAndFingerprint(t *testing.T) {
	h := startWebServer(t)
	var out bytes.Buffer
	if err := runWebURL(context.Background(), h.Client, h.StateDir, false, true, &out); err != nil {
		t.Fatal(err)
	}
	var got webURLResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseURL != hs.WebURL || !strings.HasPrefix(got.URL, hs.WebURL+"/?gx_token=") {
		t.Errorf("json = %+v, want base url %q and a tokened url", got, hs.WebURL)
	}
	if !regexp.MustCompile(`^([0-9a-f]{8} ){7}[0-9a-f]{8}$`).MatchString(got.Fingerprint) {
		t.Errorf("fingerprint = %q, want 8 groups of 8", got.Fingerprint)
	}
}
