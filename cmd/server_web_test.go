package cmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

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

// finishWebSetup sets the web account up through the real routes, so the server
// stops showing a Setup code.
func finishWebSetup(t *testing.T, h *servertest.Harness) {
	t.Helper()
	info, err := h.Client.WebInfo(context.Background())
	if err != nil || !info.SetupPending {
		t.Fatalf("web info %+v, %v; want setup pending", info, err)
	}
	client := &http.Client{Transport: servertest.WebTransport()}
	post := func(path, body string) []byte {
		req, err := http.NewRequest(http.MethodPost, info.BaseURL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", info.BaseURL)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return b
	}
	var start struct{ Secret string }
	if err := json.Unmarshal(post("/v1/web/auth/setup-start", `{"code":"`+info.SetupCode+`"}`), &start); err != nil || start.Secret == "" {
		t.Fatalf("setup-start: %v, secret %q", err, start.Secret)
	}
	totp, err := server.TOTPCode(start.Secret, time.Now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	post("/v1/web/auth/setup-finish", `{"code":"`+info.SetupCode+`","password":"correct horse battery","totp_code":"`+totp+`"}`)
	if after, _ := h.Client.WebInfo(context.Background()); after.SetupPending {
		t.Fatalf("setup still pending after setup-finish: %+v", after)
	}
}

func TestWebURL_PendingSetupPrintsTheSetupLinkAndCode(t *testing.T) {
	h := startWebServer(t)
	info, err := h.Client.WebInfo(context.Background())
	if err != nil || info.SetupCode == "" {
		t.Fatalf("web info %+v, %v", info, err)
	}
	link := info.BaseURL + "/#/setup?code=" + info.SetupCode

	var plain bytes.Buffer
	if err := runWebURL(context.Background(), h.Client, false, false, &plain); err != nil {
		t.Fatal(err)
	}
	if want := link + "\nsetup code: " + info.SetupCode + " (expires "; !strings.HasPrefix(plain.String(), want) {
		t.Errorf("web-url = %q, want prefix %q", plain.String(), want)
	}

	var qr bytes.Buffer
	if err := runWebURL(context.Background(), h.Client, true, false, &qr); err != nil {
		t.Fatal(err)
	}
	art, _, ok := strings.Cut(qr.String(), link+"\n")
	if !ok {
		t.Fatalf("web-url --qr = %q, want the setup link", qr.String())
	}
	// A code of the setup link is at least 29 modules tall plus a quiet zone; two modules fit in one text row.
	if rows := strings.Count(art, "\n"); rows < 17 || !strings.ContainsAny(art, "▀▄█") {
		t.Errorf("web-url --qr printed %d rows without block characters:\n%s", rows, qr.String())
	}
	wantArt, err := renderQR(link)
	if err != nil || art != wantArt {
		t.Errorf("web-url --qr drew a code for something other than the setup link (err %v)", err)
	}
	for _, out := range []string{plain.String(), qr.String()} {
		if strings.Contains(out, "gx_token") || strings.Contains(out, "otpauth://") {
			t.Errorf("web-url output holds a token or TOTP secret: %q", out)
		}
	}
}

func TestWebURL_AfterSetupPrintsThePlainURL(t *testing.T) {
	h := startWebServer(t)
	finishWebSetup(t, h)
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runWebURL(context.Background(), h.Client, false, false, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != hs.WebURL+"\n" {
		t.Errorf("web-url = %q, want %q", out.String(), hs.WebURL+"\n")
	}
}

func TestWebURL_UsesAnAllowedHostSetBeforeTheRootIsMade(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.WebAllowedHosts = []string{"gx.example.com"}
	})
	for _, qr := range []bool{false, true} {
		var out bytes.Buffer
		if err := runWebURL(context.Background(), h.Client, qr, false, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "https://gx.example.com:") || !strings.Contains(out.String(), "/#/setup?code=") {
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
	err := runWebURL(context.Background(), h.Client, false, false, &bytes.Buffer{})
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
	err = runWebURL(context.Background(), cl, false, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), server.ReasonServerNotRunning) {
		t.Errorf("err = %v, want %s", err, server.ReasonServerNotRunning)
	}
}

func TestWebURL_JSONCarriesBaseURLAndFingerprint(t *testing.T) {
	h := startWebServer(t)
	var out bytes.Buffer
	if err := runWebURL(context.Background(), h.Client, false, true, &out); err != nil {
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
	if got.BaseURL != hs.WebURL || got.URL != hs.WebURL+"/#/setup?code="+got.SetupCode {
		t.Errorf("json = %+v, want base url %q and the setup link", got, hs.WebURL)
	}
	if !regexp.MustCompile(`^([0-9a-f]{8} ){7}[0-9a-f]{8}$`).MatchString(got.Fingerprint) {
		t.Errorf("fingerprint = %q, want 8 groups of 8", got.Fingerprint)
	}
}

// webClientTrustingServedRoot trusts only the root the server serves at .cer right now.
func webClientTrustingServedRoot(t *testing.T, baseURL string) *http.Client {
	t.Helper()
	bootstrap := &http.Client{Transport: &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	resp, err := bootstrap.Get(baseURL + "/gx-root.cer")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	der, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("the .cer route did not serve a certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return &http.Client{Transport: &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{RootCAs: pool}}}
}

func TestWebNewRoot_SwapsRootAndLeafLive(t *testing.T) {
	h := startWebServer(t)
	hs, err := h.Client.Handshake(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	oldClient := webClientTrustingServedRoot(t, hs.WebURL)
	resp, err := oldClient.Get(hs.WebURL + "/gx-root.cer")
	if err != nil {
		t.Fatalf("old root before the swap: %v", err)
	}
	resp.Body.Close()
	before, err := h.Client.WebInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWebNewRoot(context.Background(), h.Client, true, &out); err != nil {
		t.Fatal(err)
	}
	var got server.WebInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if got.Fingerprint == "" || got.Fingerprint == before.Fingerprint {
		t.Errorf("fingerprint = %q, want one different from %q", got.Fingerprint, before.Fingerprint)
	}
	after, err := h.Client.WebInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after.Fingerprint != got.Fingerprint {
		t.Errorf("web-url fingerprint = %q, want the new one %q", after.Fingerprint, got.Fingerprint)
	}

	if resp, err := oldClient.Get(hs.WebURL + "/gx-root.cer"); err == nil {
		resp.Body.Close()
		t.Error("a client trusting only the old root still completes the handshake")
	}
	resp, err = webClientTrustingServedRoot(t, hs.WebURL).Get(hs.WebURL + "/gx-root.cer")
	if err != nil {
		t.Fatalf("new root after the swap: %v", err)
	}
	resp.Body.Close()
}

func TestWebNewRoot_RefusesWhenTheWebUIIsOff(t *testing.T) {
	h := servertest.StartWithStore(t, t.TempDir(), func(*server.Config) {})
	err := runWebNewRoot(context.Background(), h.Client, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "the web UI is off") {
		t.Errorf("err = %v, want the web UI is off", err)
	}
}
