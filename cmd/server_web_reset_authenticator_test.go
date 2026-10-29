package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

// secretLeak matches a base32 secret of the stored length or an otpauth:// URI.
var secretLeak = regexp.MustCompile(`otpauth://|\b[A-Z2-7]{32}\b`)

func postLogin(t *testing.T, baseURL, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/web/auth/login", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Transport: servertest.WebTransport()}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestWebResetAuthenticator_PrintsASetupCodeAndNoSecret(t *testing.T) {
	for _, jsonFlag := range []bool{false, true} {
		h := startWebServer(t)
		old, secret := finishWebSetupWithSecret(t, h)
		base := mustWebInfo(t, h).BaseURL

		args := []string{"server", "web-reset-authenticator"}
		if jsonFlag {
			args = append(args, "--json")
		}
		var out bytes.Buffer
		if err := execute(args, webResetDeps(t, h, "", false, &out)); err != nil {
			t.Fatal(err)
		}
		if secretLeak.MatchString(out.String()) || strings.Contains(out.String(), secret) {
			t.Errorf("output holds a TOTP secret:\n%s", out.String())
		}
		code := mustWebInfo(t, h).SetupCode
		if code == "" || !strings.Contains(out.String(), code) {
			t.Fatalf("output %q does not hold the Setup code %q", out.String(), code)
		}

		var status bytes.Buffer
		if err := runServerStatusWith(context.Background(), config.DirsFrom(tempEnv(t)), h.Client, true, &status); err != nil {
			t.Fatal(err)
		}
		var st StatusInfo
		if err := json.Unmarshal(status.Bytes(), &st); err != nil || st.WebSetupCode != code {
			t.Errorf("status --json web_setup_code = %q (err %v), want %q", st.WebSetupCode, err, code)
		}
		var url bytes.Buffer
		if err := runWebURL(context.Background(), h.Client, false, false, &url); err != nil || !strings.Contains(url.String(), code) {
			t.Errorf("web-url = %q (err %v), want the Setup code %q", url.String(), err, code)
		}

		if got := webLoginsStatus(t, base, old); got != http.StatusUnauthorized {
			t.Errorf("old cookie after reset = %d, want 401", got)
		}
		if list, err := h.Client.WebLogins(context.Background()); err != nil || len(list.Logins) != 0 {
			t.Errorf("logins after reset = %+v, %v; want none", list, err)
		}

		oldCode, _ := server.TOTPCode(secret, time.Now().Unix()/30)
		_, body := postLogin(t, base, `{"password":"correct horse battery","totp_code":"`+oldCode+`"}`)
		if !strings.Contains(body, server.ReasonSetupCodeRequired) {
			t.Errorf("login without the Setup code = %s, want %s", body, server.ReasonSetupCodeRequired)
		}
		_, body = postLogin(t, base, `{"password":"correct horse battery","setup_code":"`+code+`"}`)
		if !strings.Contains(body, `"authenticator_needed":true`) {
			t.Errorf("login with the Setup code = %s, want authenticator needed", body)
		}
	}
}

func TestWebResetAuthenticator_NoServerIsServerNotRunning(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "gxn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	err = runWebResetAuthenticator(context.Background(), apiclient.New(filepath.Join(dir, "none.sock")), false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), server.ReasonServerNotRunning) {
		t.Errorf("err = %v, want %s", err, server.ReasonServerNotRunning)
	}
}

func TestWebResetAuthenticator_RefusesWhenTheWebUIIsOff(t *testing.T) {
	h := servertest.Start(t)
	err := runWebResetAuthenticator(context.Background(), h.Client, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "the web UI is off") {
		t.Errorf("err = %v, want the web UI is off", err)
	}
}

// Story 113: no gx server web command prints a TOTP secret or an otpauth:// URI.
func TestWebCommands_NeverPrintATOTPSecret(t *testing.T) {
	commands := [][]string{
		{"server", "web-url"}, {"server", "web-url", "--json"},
		{"server", "web-logins"}, {"server", "web-logins", "--json"},
		{"server", "status"}, {"server", "status", "--json"},
	}
	check := func(t *testing.T, h *servertest.Harness, stage string) {
		for _, args := range commands {
			var out bytes.Buffer
			d := webResetDeps(t, h, "", false, &out)
			if err := execute(args, d); err != nil {
				t.Fatalf("%s: %v: %v", stage, args, err)
			}
			if secretLeak.MatchString(out.String()) {
				t.Errorf("%s: %v printed a secret:\n%s", stage, args, out.String())
			}
		}
	}
	h := startWebServer(t)
	check(t, h, "setup pending")
	finishWebSetup(t, h)
	check(t, h, "after setup")
	var out bytes.Buffer
	if err := execute([]string{"server", "web-reset-authenticator", "--json"}, webResetDeps(t, h, "", false, &out)); err != nil {
		t.Fatal(err)
	}
	check(t, h, "after authenticator reset")
}
