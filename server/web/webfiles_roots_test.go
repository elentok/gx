package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire/ops"
)

func TestFilesRoots_WebNeedsCookieAndAddMakesAFileFound(t *testing.T) {
	t.Parallel()
	h := servertest.StartWithStore(t, t.TempDir(),
		servertest.WithConfigFile(filepath.Join(t.TempDir(), "config.json")),
		func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	hs, err := h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	base, cookie := hs.WebURL, servertest.Login(t, h.StateDir, hs.WebURL)

	dir := servertest.OutsideTempDir(t)
	file := filepath.Join(dir, "a.md")
	if err := os.WriteFile(file, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	post := func(path, body string, c *http.Cookie) (int, ops.HostedRootsResult) {
		code, out := servertest.WebPostJSON(t, base+path, body, c, "", servertest.JSONPost(base))
		var res ops.HostedRootsResult
		if code == http.StatusOK {
			if err := json.Unmarshal([]byte(out), &res); err != nil {
				t.Fatalf("%s body %q: %v", path, out, err)
			}
		}
		return code, res
	}
	resolved := func() ops.HostedVerdict {
		body, _ := json.Marshal(ops.HostedResolveRequest{Candidates: []string{file}})
		code, out := servertest.WebPostJSON(t, base+"/v1/hosted-files/resolve", string(body), cookie, "", servertest.JSONPost(base))
		var res ops.HostedResolveResult
		if code != http.StatusOK || json.Unmarshal([]byte(out), &res) != nil || len(res.Files) != 1 {
			t.Fatalf("resolve = %d %s", code, out)
		}
		return res.Files[0].Verdict
	}

	body := `{"path":"` + dir + `"}`
	for _, p := range []string{"/v1/hosted-files/roots/add", "/v1/hosted-files/roots/remove"} {
		if code, _ := post(p, body, nil); code == http.StatusOK {
			t.Errorf("POST %s without the cookie = %d, want a refusal", p, code)
		}
	}
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, base+"/v1/hosted-files/roots", nil); code == http.StatusOK {
		t.Errorf("GET roots without the cookie = %d", code)
	}
	if got := resolved(); got != ops.HostedMissing {
		t.Fatalf("before add: %q", got)
	}

	code, res := post("/v1/hosted-files/roots/add", body, cookie)
	if code != http.StatusOK || res.Refused {
		t.Fatalf("add = %d %+v", code, res)
	}
	var listed bool
	for _, r := range res.Roots {
		listed = listed || (r.Path == dir && r.Source == ops.HostedRootConfig)
	}
	if !listed {
		t.Errorf("roots %+v lack %s with source config", res.Roots, dir)
	}
	if got := resolved(); got != ops.HostedFound {
		t.Errorf("after add: %q, want found", got)
	}

	code, out := servertest.WebDoCookie(t, http.MethodGet, base+"/v1/hosted-files/roots", cookie)
	if code != http.StatusOK {
		t.Fatalf("GET roots = %d %s", code, out)
	}

	if _, res := post("/v1/hosted-files/roots/remove", body, cookie); res.Refused {
		t.Fatalf("remove refused: %+v", res)
	}
	if got := resolved(); got != ops.HostedMissing {
		t.Errorf("after remove: %q, want missing", got)
	}
}
