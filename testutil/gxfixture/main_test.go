package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server"
)

// smokeScenarios are the scenarios no other test starts, plus "ready" as the
// baseline. The covered ones need no second start here.
var smokeScenarios = []string{
	"check-lock", "design-branch", "design-defaulted", "design-error", "design-guard",
	"design-mixed", "design-plain", "design-text", "design-working", "edge",
	"long-running", "mixed", "model-mismatch", "restart-launchd", "ready",
}

func TestScenarios_StartAnswerAndStop(t *testing.T) {
	t.Parallel()
	for _, name := range smokeScenarios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := start(t, name)
			if r.StateDir == "" || r.HoldFile == "" || r.Socket == "" {
				t.Fatalf("output = %+v, want state dir, socket and hold file", r.Output)
			}
			if !strings.Contains(r.WebURL, "127.0.0.1") || strings.Contains(r.WebURL, "?") {
				t.Fatalf("web_url = %q, want a plain loopback URL", r.WebURL)
			}
			// These two start signed out, and have their own tests.
			signedOut := name == "web-setup" || name == "web-login"
			if !signedOut && (r.Cookie == "" || r.TOTPSecret == "" || r.TOTPStep == 0) {
				t.Fatalf("output = %+v, want a cookie, a TOTP secret and the last step", r.Output)
			}
			snapshot := r.WebURL + "/v1/snapshot"
			if code := r.get(t, snapshot, true); !signedOut && code != http.StatusOK {
				t.Errorf("GET /v1/snapshot with the cookie = %d, want 200", code)
			}
			if code := r.get(t, snapshot, false); code == http.StatusOK {
				t.Errorf("GET /v1/snapshot without the cookie = %d, want it refused", code)
			}
			var st struct {
				Running bool `json:"running"`
			}
			if err := json.Unmarshal([]byte(r.gx(t, "server", "status", "--json")), &st); err != nil || !st.Running {
				t.Fatalf("server status = %+v, %v; want running", st, err)
			}
			if _, err := os.Stat(r.Socket); err != nil {
				t.Errorf("printed socket: %v", err)
			}

			r.stop(t)
			if _, err := os.Stat(filepath.Dir(filepath.Dir(r.StateDir))); !os.IsNotExist(err) {
				t.Errorf("temp dir after SIGTERM: %v, want it removed", err)
			}
			if err := exec.Command("pgrep", "-f", filepath.Dir(filepath.Dir(r.StateDir))).Run(); err == nil {
				t.Errorf("a process still names the temp dir")
			}
		})
	}
}

func TestFixture_TOTPOffSignsInWithoutASecret(t *testing.T) {
	t.Parallel()
	r := startWith(t, "ready", nil, "--config", "server.web.totp=false")
	if r.Cookie == "" {
		t.Fatalf("output = %+v, want a cookie", r.Output)
	}
	if r.TOTPSecret != "" || r.TOTPStep != 0 {
		t.Errorf("TOTP secret = %q, step = %d; want none with server.web.totp=false", r.TOTPSecret, r.TOTPStep)
	}
	if code := r.get(t, r.WebURL+"/v1/snapshot", true); code != http.StatusOK {
		t.Errorf("GET /v1/snapshot with the cookie = %d, want 200", code)
	}
	r.stop(t)
}

func TestWebSetupScenario_LeavesTheSetupPendingAndSignedOut(t *testing.T) {
	t.Parallel()
	r := start(t, "web-setup")
	if r.Cookie != "" || r.Password != "" || r.TOTPSecret != "" {
		t.Errorf("output = %+v, want no cookie, password or secret", r.Output)
	}
	if code := r.get(t, r.WebURL+"/v1/snapshot", false); code != http.StatusUnauthorized {
		t.Errorf("GET /v1/snapshot = %d, want 401", code)
	}
	var st struct {
		WebSetupCode string `json:"web_setup_code"`
	}
	if err := json.Unmarshal([]byte(r.gx(t, "server", "status", "--json")), &st); err != nil || st.WebSetupCode == "" {
		t.Errorf("server status = %+v, %v; want a web_setup_code", st, err)
	}
	r.stop(t)
}

func TestWebLoginScenario_LogsInWithItsPasswordAndSecret(t *testing.T) {
	t.Parallel()
	for _, totpOff := range []bool{false, true} {
		var flags []string
		if totpOff {
			flags = []string{"--config", "server.web.totp=false"}
		}
		r := startWith(t, "web-login", nil, flags...)
		if r.Cookie != "" || r.Password == "" {
			t.Fatalf("totp off=%v: output = %+v, want a password and no cookie", totpOff, r.Output)
		}
		if (r.TOTPSecret == "") != totpOff {
			t.Fatalf("totp off=%v: secret = %q", totpOff, r.TOTPSecret)
		}
		body := map[string]string{"password": r.Password}
		if !totpOff {
			code, err := server.TOTPCode(r.TOTPSecret, r.TOTPStep+1)
			if err != nil {
				t.Fatal(err)
			}
			body["totp_code"] = code
		}
		client := r.webClient(t, false)
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, r.WebURL+"/v1/web/auth/login", strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", r.WebURL)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || strings.Contains(string(data), `"refused"`) {
			t.Fatalf("totp off=%v: login = %d %s, want it accepted", totpOff, resp.StatusCode, data)
		}
		if code := func() int {
			resp, err := client.Get(r.WebURL + "/v1/snapshot")
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			return resp.StatusCode
		}(); code != http.StatusOK {
			t.Errorf("totp off=%v: GET /v1/snapshot after login = %d, want 200", totpOff, code)
		}
		r.stop(t)
	}
}

func TestWebLoginsScenario_HasFourLoginsOneIdleSeventyDays(t *testing.T) {
	t.Parallel()
	r := start(t, "web-logins")
	if code := r.get(t, r.WebURL+"/v1/snapshot", true); code != http.StatusOK {
		t.Errorf("GET /v1/snapshot with the cookie = %d, want 200", code)
	}
	auth, err := server.OpenWebAuth(r.StateDir, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	logins := auth.List()
	if len(logins) != 4 {
		t.Fatalf("web logins = %d, want 4: %+v", len(logins), logins)
	}
	idle := 0
	for _, l := range logins {
		if time.Since(l.LastUsed) > 69*24*time.Hour {
			idle++
		}
	}
	if idle != 1 {
		t.Errorf("logins idle for 69+ days = %d, want 1", idle)
	}
	r.stop(t)
}
