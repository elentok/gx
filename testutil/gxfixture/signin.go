package main

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

	"github.com/elentok/gx/server"
)

// fixturePassword is the password every fixture's web login is set up with.
const fixturePassword = "gxfixture-password"

// signIn is the web sign-in gxfixture finished: the login cookie, and, when the
// server asks for a code, the TOTP secret and the step of the last code used.
type signIn struct {
	cookieName, cookie, totpSecret string
	totpStep                       int64
}

// webSetup finishes the web setup through the real routes, with the Setup code
// from `gx server status --json`, as a person's first visit would.
func webSetup(stateDir, webURL, setupCode string) (signIn, error) {
	root, err := os.ReadFile(filepath.Join(stateDir, "web-tls", "root.pem"))
	if err != nil {
		return signIn{}, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(root) {
		return signIn{}, fmt.Errorf("%s has no certificate", filepath.Join(stateDir, "web-tls", "root.pem"))
	}
	u, err := url.Parse(webURL)
	if err != nil {
		return signIn{}, err
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
		return signIn{}, err
	}
	var state struct {
		TOTPOn bool `json:"totp_on"`
	}
	err = json.NewDecoder(resp.Body).Decode(&state)
	resp.Body.Close()
	if err != nil {
		return signIn{}, fmt.Errorf("GET /v1/web/auth: %w", err)
	}

	var in signIn
	var started struct {
		Secret string `json:"secret"`
	}
	if _, err := post("/v1/web/auth/setup-start", map[string]string{"code": setupCode}, &started); err != nil {
		return signIn{}, err
	}
	finish := map[string]string{"code": setupCode, "password": fixturePassword}
	if state.TOTPOn {
		if started.Secret == "" {
			return signIn{}, fmt.Errorf("setup step 1 returned no TOTP secret")
		}
		in.totpSecret, in.totpStep = started.Secret, server.TOTPStepAt(time.Now())
		if finish["totp_code"], err = server.TOTPCode(in.totpSecret, in.totpStep); err != nil {
			return signIn{}, err
		}
	}
	resp, err = post("/v1/web/auth/setup-finish", finish, &struct{}{})
	if err != nil {
		return signIn{}, err
	}
	for _, c := range resp.Cookies() {
		if strings.HasPrefix(c.Name, "__Host-gx_login") {
			in.cookieName, in.cookie = c.Name, c.Value
		}
	}
	if in.cookie == "" {
		return signIn{}, fmt.Errorf("setup finished without a login cookie")
	}
	return in, nil
}
