package files

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/server"
)

func startWeb(t *testing.T) fixture {
	t.Helper()
	return start(t, func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.WebAllowedHosts = []string{"gx.example.com"}
	})
}

var viewerLink = regexp.MustCompile(`^https://gx\.example\.com:\d+/#/files(/.+)$`)

func TestFilesURL_ViewerLinkFromRepoAndSubfolder(t *testing.T) {
	f := startWeb(t)
	want, err := filepath.EvalSymlinks(filepath.Join(f.repo, "docs/plans/host-files.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct{ cwd, arg string }{
		"repo":      {f.repo, "docs/plans/host-files.md"},
		"subfolder": {filepath.Join(f.repo, "docs"), "plans/host-files.md"},
		"absolute":  {t.TempDir(), want},
	} {
		out, err := f.run(t, tc.cwd, "url", tc.arg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		m := viewerLink.FindStringSubmatch(strings.TrimSpace(out))
		if m == nil || m[1] != want {
			t.Errorf("%s: out = %q, want viewer link for %s", name, out, want)
		}
	}
}

func TestFilesURL_SpaceIsPercentEncoded(t *testing.T) {
	f := startWeb(t)
	if err := os.WriteFile(filepath.Join(f.repo, "my notes.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := f.run(t, f.repo, "url", "my notes.md")
	if err != nil || !strings.HasSuffix(strings.TrimSpace(out), "/my%20notes.md") {
		t.Errorf("out = %q err = %v", out, err)
	}
}

func TestFilesURL_RawLink(t *testing.T) {
	f := startWeb(t)
	out, err := f.run(t, f.repo, "url", "--raw", "docs/plans/host-files.md")
	if err != nil || !regexp.MustCompile(`^https://gx\.example\.com:\d+/v1/files/.+/docs/plans/host-files\.md\n$`).MatchString(out) {
		t.Errorf("out = %q err = %v", out, err)
	}
}

func TestFilesURL_Host(t *testing.T) {
	f := startWeb(t)
	for host, prefix := range map[string]string{"localhost": "https://localhost:", "gx.example.com": "https://gx.example.com:", "10.1.2.3": "https://10.1.2.3:"} {
		out, err := f.run(t, f.repo, "url", "--host", host, "docs/plans/host-files.md")
		if err != nil || !strings.HasPrefix(out, prefix) {
			t.Errorf("host %s: out = %q err = %v", host, out, err)
		}
	}
	_, err := f.run(t, f.repo, "url", "--host", "evil.example", "docs/plans/host-files.md")
	if err == nil || !strings.Contains(err.Error(), "host-not-allowed") {
		t.Errorf("err = %v, want host-not-allowed", err)
	}
	out, err := f.run(t, f.repo, "url", "--json", "--host", "evil.example", "docs/plans/host-files.md")
	cmdtest.RequireRefusalExit(t, err)
	var j struct {
		Refused bool   `json:"refused"`
		Reason  string `json:"reason"`
	}
	if e := json.Unmarshal([]byte(out), &j); e != nil || !j.Refused || j.Reason != "host-not-allowed" {
		t.Errorf("json = %q (%v)", out, e)
	}
}

func TestFilesURL_DeniedPathsShareOneRefusal(t *testing.T) {
	f := startWeb(t)
	var messages []string
	for _, arg := range []string{"docs/none.md", ".env", "docs", "/etc/passwd"} {
		_, err := f.run(t, f.repo, "url", arg)
		if err == nil || !strings.Contains(err.Error(), "path-not-allowed") {
			t.Fatalf("%s: err = %v, want path-not-allowed", arg, err)
		}
		messages = append(messages, err.Error())
	}
	for _, m := range messages[1:] {
		if m != messages[0] {
			t.Errorf("message %q differs from %q", m, messages[0])
		}
	}
}

func TestFilesURL_FileInTempFolderSucceeds(t *testing.T) {
	f := startWeb(t)
	p := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := f.run(t, f.repo, "url", p); err != nil || !strings.Contains(out, "/#/files/") {
		t.Errorf("out = %q err = %v", out, err)
	}
}

func TestFilesURL_WebOffMatchesWebURLRefusal(t *testing.T) {
	f := start(t)
	_, err := f.run(t, f.repo, "url", "docs/plans/host-files.md")
	info, ierr := f.h.Client.Web.Info(context.Background())
	if err == nil || ierr != nil || !info.Refused {
		t.Fatalf("err = %v, info = %+v (%v)", err, info, ierr)
	}
	if want := "refused (" + info.Reason + "): " + info.Message; err.Error() != want {
		t.Errorf("err = %q, want %q", err, want)
	}
}
