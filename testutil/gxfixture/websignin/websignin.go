// Package websignin finishes gxfixture's web setup through the server's real routes.
package websignin

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/server/webauth"
)

// Password is the password every fixture's web login is set up with.
const Password = "gxfixture-password"

// SignIn is the web sign-in gxfixture finished: the login cookie, and, when the
// server asks for a code, the TOTP secret and the step of the last code used.
type SignIn struct {
	CookieName, Cookie, TOTPSecret string
	TOTPStep                       int64
}

// Setup finishes the web setup through the real routes, with the Setup code
// from `gx server status --json`, as a person's first visit would.
func Setup(stateDir, webURL, setupCode string) (SignIn, error) {
	root, err := os.ReadFile(filepath.Join(stateDir, "web-tls", "root.pem"))
	if err != nil {
		return SignIn{}, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(root) {
		return SignIn{}, fmt.Errorf("%s has no certificate", filepath.Join(stateDir, "web-tls", "root.pem"))
	}
	u, err := url.Parse(webURL)
	if err != nil {
		return SignIn{}, err
	}
	origin := u.Scheme + "://" + u.Host
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}

	post := func(path string, body any, out any) (*http.Response, error) {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequest(http.MethodPost, origin+path, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		var res struct {
			Refused bool   `json:"refused"`
			Reason  string `json:"reason"`
		}
		if resp.StatusCode != http.StatusOK || json.Unmarshal(data, &res) != nil || res.Refused {
			return nil, fmt.Errorf("POST %s: %d %s", path, resp.StatusCode, data)
		}
		return resp, json.Unmarshal(data, out)
	}

	// The fixture follows what the server does, not what it asked the config for.
	resp, err := client.Get(origin + "/v1/web/auth")
	if err != nil {
		return SignIn{}, err
	}
	var state struct {
		TOTPOn bool `json:"totp_on"`
	}
	err = json.NewDecoder(resp.Body).Decode(&state)
	resp.Body.Close()
	if err != nil {
		return SignIn{}, fmt.Errorf("GET /v1/web/auth: %w", err)
	}

	var in SignIn
	var started struct {
		Secret string `json:"secret"`
	}
	if _, err := post("/v1/web/auth/setup-start", map[string]string{"code": setupCode}, &started); err != nil {
		return SignIn{}, err
	}
	finish := map[string]string{"code": setupCode, "password": Password}
	if state.TOTPOn {
		if started.Secret == "" {
			return SignIn{}, fmt.Errorf("setup step 1 returned no TOTP secret")
		}
		// The server accepts the step before its own, so the setup code uses it and leaves two
		// steps for the flows to use without waiting. Near the end of a step the server's clock
		// could already be on the next one, which no longer accepts the previous.
		now := time.Now()
		in.TOTPSecret, in.TOTPStep = started.Secret, webauth.TOTPStepAt(now)
		if now.Unix()%30 < 25 {
			in.TOTPStep--
		}
		if finish["totp_code"], err = webauth.TOTPCode(in.TOTPSecret, in.TOTPStep); err != nil {
			return SignIn{}, err
		}
	}
	resp, err = post("/v1/web/auth/setup-finish", finish, &struct{}{})
	if err != nil {
		return SignIn{}, err
	}
	for _, c := range resp.Cookies() {
		if strings.HasPrefix(c.Name, "__Host-gx_login") {
			in.CookieName, in.Cookie = c.Name, c.Value
		}
	}
	if in.Cookie == "" {
		return SignIn{}, fmt.Errorf("setup finished without a login cookie")
	}
	return in, nil
}
