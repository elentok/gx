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
