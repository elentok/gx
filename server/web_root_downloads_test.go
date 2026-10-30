package server_test

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWebRoot_DownloadsNeedNoLoginAndCarryNoHSTS(t *testing.T) {
	t.Parallel()
	h, base := startWeb(t)
	root, _ := webRootPEM(t, h)
	tests := []struct {
		path, contentType string
		check             func(t *testing.T, body []byte)
	}{
		{"/gx-root.mobileconfig", "application/x-apple-aspen-config", func(t *testing.T, body []byte) {
			if !strings.Contains(string(body), "com.apple.security.root") {
				t.Errorf("profile has no root payload:\n%s", body)
			}
		}},
		{"/gx-root.cer", "application/pkix-cert", func(t *testing.T, body []byte) {
			if !bytes.Equal(body, root.Raw) {
				t.Error("body is not the root's DER")
			}
		}},
	}
	for _, tt := range tests {
		resp, err := webClient().Get(base + tt.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d, want 200", tt.path, resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); got != tt.contentType {
			t.Errorf("%s: content type %q, want %q", tt.path, got, tt.contentType)
		}
		if len(resp.Cookies()) != 0 {
			t.Errorf("%s: set a cookie", tt.path)
		}
		if got := resp.Header.Get("Strict-Transport-Security"); got != "" {
			t.Errorf("%s: HSTS header %q", tt.path, got)
		}
		tt.check(t, body)
	}
}

func TestWebHead_PublicRoutesMatchGetWithoutBody(t *testing.T) {
	t.Parallel()
	_, base := startWeb(t)
	for _, path := range []string{"/gx-root.mobileconfig", "/gx-root.cer", "/"} {
		get, err := webClient().Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		get.Body.Close()
		resp, err := webClient().Head(base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || get.StatusCode != http.StatusOK {
			t.Errorf("%s: HEAD %d, GET %d, want 200", path, resp.StatusCode, get.StatusCode)
		}
		for _, name := range []string{"Content-Type", "Cache-Control", "Strict-Transport-Security"} {
			if got, want := resp.Header.Get(name), get.Header.Get(name); got != want {
				t.Errorf("%s: HEAD %s %q, GET %q", path, name, got, want)
			}
		}
	}
}

func TestWebHead_NeedsLoginOutsidePublicRoutes(t *testing.T) {
	t.Parallel()
	_, base := startWeb(t)
	for _, path := range []string{"/v1/snapshot", "/v1/events"} {
		resp, err := webClient().Head(base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("HEAD %s = %d, want 401", path, resp.StatusCode)
		}
	}
	resp, err := webClient().Head(base + "/assets/missing.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("HEAD /assets/missing.js = %d, want 404 from the asset handler, not the login gate", resp.StatusCode)
	}
}
