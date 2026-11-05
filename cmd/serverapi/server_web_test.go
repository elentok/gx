package serverapi

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

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

func startWebServer(t *testing.T) *servertest.Harness {
	t.Helper()
	return servertest.StartWithStore(t, t.TempDir(), func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
}

func TestServerWebURL_NoHomeInEnvIsAnError(t *testing.T) {
	d := Deps{Env: cmdbase.Env{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}}
	err := execute(t, []string{"server", "web-url"}, d)
	if err == nil || !strings.Contains(err.Error(), "no HOME in env") {
		t.Errorf("err = %v, want it to mention no HOME in env", err)
	}
}

// webLoginsStatus is the status the web logins list answers an old or new cookie with.
func webLoginsStatus(t *testing.T, baseURL string, cookie *http.Cookie) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/web/logins", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp, err := (&http.Client{Transport: servertest.WebTransport()}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func webResetDeps(t *testing.T, h *servertest.Harness, stdin string, tty bool, out *bytes.Buffer) Deps {
	return Deps{
		Env: cmdbase.Env{
			Env:       cmdtest.TempEnv(t),
			Stdout:    out,
			Stderr:    &bytes.Buffer{},
			Stdin:     strings.NewReader(stdin),
			NewClient: func() (*apiclient.Client, error) { return h.Client, nil },
		},
		StdinIsTerminal: func() bool { return tty },
	}
}

func TestWebReset_YesClearsEverythingAndPrintsANewCode(t *testing.T) {
	h := startWebServer(t)
	old := finishWebSetup(t, h)
	if got := webLoginsStatus(t, mustWebInfo(t, h).BaseURL, old); got != http.StatusOK {
		t.Fatalf("logins before reset = %d, want 200", got)
	}

	var out bytes.Buffer
	if err := execute(t, []string{"server", "web-reset", "--yes", "--json"}, webResetDeps(t, h, "", false, &out)); err != nil {
		t.Fatal(err)
	}
	var got wire.WebInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if !got.SetupPending || got.SetupCode == "" {
		t.Errorf("result = %+v, want a pending setup with a new code", got)
	}

	if code := mustWebInfo(t, h).SetupCode; code != got.SetupCode {
		t.Errorf("web info setup code = %q, want %q", code, got.SetupCode)
	}
	if got := webLoginsStatus(t, mustWebInfo(t, h).BaseURL, old); got != http.StatusUnauthorized {
		t.Errorf("old cookie after reset = %d, want 401", got)
	}
	logs, _ := os.ReadFile(server.LogPath(h.StateDir))
	if !strings.Contains(string(logs), `"msg":"web reset","ip":"cli"`) {
		t.Errorf("gx.log has no web reset line for ip cli:\n%s", logs)
	}
}

func mustWebInfo(t *testing.T, h *servertest.Harness) wire.WebInfo {
	t.Helper()
	info, err := h.Client.Web.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestWebReset_WithoutATerminalOrYesRefusesAndChangesNothing(t *testing.T) {
	h := startWebServer(t)
	old := finishWebSetup(t, h)
	var out bytes.Buffer
	err := execute(t, []string{"server", "web-reset"}, webResetDeps(t, h, "y\n", false, &out))
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("err = %v, want a refusal naming --yes", err)
	}
	if got := webLoginsStatus(t, mustWebInfo(t, h).BaseURL, old); got != http.StatusOK {
		t.Errorf("old cookie after refused reset = %d, want 200", got)
	}
}

func TestWebReset_AsksOnATerminal(t *testing.T) {
	for _, tc := range []struct {
		answer    string
		wantReset bool
	}{{"y\n", true}, {"n\n", false}, {"\n", false}} {
		t.Run(strings.TrimSpace(tc.answer)+"-enter", func(t *testing.T) {
			h := startWebServer(t)
			old := finishWebSetup(t, h)
			var out bytes.Buffer
			if err := execute(t, []string{"server", "web-reset"}, webResetDeps(t, h, tc.answer, true, &out)); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "[y/N]") {
				t.Errorf("output = %q, want a y/N prompt", out.String())
			}
			want := http.StatusOK
			if tc.wantReset {
				want = http.StatusUnauthorized
			}
			if got := webLoginsStatus(t, mustWebInfo(t, h).BaseURL, old); got != want {
				t.Errorf("old cookie = %d, want %d", got, want)
			}
		})
	}
}

func TestWebReset_NoServerIsServerNotRunning(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "gxn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	err = runWebReset(context.Background(), apiclient.New(filepath.Join(dir, "none.sock")), false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), wire.ReasonServerNotRunning) {
		t.Errorf("err = %v, want %s", err, wire.ReasonServerNotRunning)
	}
}

func TestWebReset_RefusesWhenTheWebUIIsOff(t *testing.T) {
	h := servertest.Start(t)
	err := runWebReset(context.Background(), h.Client, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "the web UI is off") {
		t.Errorf("err = %v, want the web UI is off", err)
	}
}

func TestWebURL_PendingSetupPrintsTheSetupLinkAndCode(t *testing.T) {
	h := startWebServer(t)
	info, err := h.Client.Web.Info(context.Background())
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
	if err == nil || !strings.Contains(err.Error(), wire.ReasonServerNotRunning) {
		t.Errorf("err = %v, want %s", err, wire.ReasonServerNotRunning)
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
	before, err := h.Client.Web.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runWebNewRoot(context.Background(), h.Client, true, &out); err != nil {
		t.Fatal(err)
	}
	var got wire.WebInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if got.Fingerprint == "" || got.Fingerprint == before.Fingerprint {
		t.Errorf("fingerprint = %q, want one different from %q", got.Fingerprint, before.Fingerprint)
	}
	after, err := h.Client.Web.Info(context.Background())
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
