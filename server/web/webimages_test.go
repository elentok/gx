package web_test

import (
	"net/http"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

// The image upload route is the one web write whose body is not JSON: it keeps
// the Origin and login checks and drops only the content-type rule.
func TestWeb_ImageUploadSkipsOnlyTheJSONRule(t *testing.T) {
	t.Parallel()
	h, base := servertest.StartWeb(t)
	const upload, prompt = "/v1/sessions/images", "/v1/sessions/prompt"
	raw := map[string]string{"Origin": base, "Content-Type": "application/octet-stream"}
	for name, tc := range map[string]struct {
		path    string
		headers map[string]string
		want    int
	}{
		"upload, non-JSON type":    {upload, raw, http.StatusOK},
		"upload, no type":          {upload, map[string]string{"Origin": base, "Content-Type": ""}, http.StatusOK},
		"upload, no Origin":        {upload, map[string]string{"Content-Type": "image/png"}, http.StatusForbidden},
		"upload, wrong Origin":     {upload, map[string]string{"Origin": "https://evil.example", "Content-Type": "image/png"}, http.StatusForbidden},
		"prompt keeps JSON rule":   {prompt, raw, http.StatusForbidden},
		"prompt with JSON is open": {prompt, map[string]string{"Origin": base, "Content-Type": "application/json"}, http.StatusOK},
	} {
		if got := webSend(t, h, http.MethodPost, base+tc.path, tc.headers); got != tc.want {
			t.Errorf("%s = %d, want %d", name, got, tc.want)
		}
	}
	if code, _ := servertest.WebDoCookie(t, http.MethodPost, base+upload, nil); code != http.StatusUnauthorized {
		t.Errorf("signed-out upload = %d, want %d", code, http.StatusUnauthorized)
	}
}
