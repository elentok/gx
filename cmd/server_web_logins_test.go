package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestWebLogins_JSONListsEveryLoginWithItsFiveFields(t *testing.T) {
	h := startWebServer(t)
	finishWebSetup(t, h)

	var out bytes.Buffer
	if err := execute([]string{"server", "web-logins", "--json"}, webResetDeps(t, h, "", false, &out)); err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Logins []map[string]any `json:"logins"`
	}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(raw.Logins) != 1 {
		t.Fatalf("logins = %v, want 1", raw.Logins)
	}
	for _, key := range []string{"id", "name", "created", "last_used", "last_ip"} {
		if _, ok := raw.Logins[0][key]; !ok {
			t.Errorf("login %v has no %q", raw.Logins[0], key)
		}
	}
}

func TestWebRevoke_EndsTheOpenEventStreamAndLogsLikeAWebRevoke(t *testing.T) {
	h := startWebServer(t)
	cookie := finishWebSetup(t, h)
	list, err := h.Client.WebLogins(context.Background())
	if err != nil || len(list.Logins) != 1 {
		t.Fatalf("logins %+v, %v", list, err)
	}
	id, name := list.Logins[0].ID, list.Logins[0].Name

	req, _ := http.NewRequest(http.MethodGet, mustWebInfo(t, h).BaseURL+"/v1/events", nil)
	req.AddCookie(cookie)
	stream, err := (&http.Client{Transport: servertest.WebTransport()}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	ended := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, stream.Body); close(ended) }()

	var out bytes.Buffer
	if err := execute([]string{"server", "web-revoke", id}, webResetDeps(t, h, "", false, &out)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Error("the event stream stayed open after the CLI revoke")
	}
	if got := webLoginsStatus(t, mustWebInfo(t, h).BaseURL, cookie); got != http.StatusUnauthorized {
		t.Errorf("cookie after revoke = %d, want 401", got)
	}
	if after, _ := h.Client.WebLogins(context.Background()); len(after.Logins) != 0 {
		t.Errorf("logins after revoke = %+v, want none", after.Logins)
	}
	logs, _ := os.ReadFile(server.LogPath(h.StateDir))
	want := `"msg":"web login revoked","ip":"cli","name":"` + name + `"`
	if !strings.Contains(string(logs), want) {
		t.Errorf("gx.log has no %s line:\n%s", want, logs)
	}
}

func TestWebRevoke_UnknownIDIsRefusedWithItsReason(t *testing.T) {
	h := startWebServer(t)
	finishWebSetup(t, h)
	err := runWebRevoke(context.Background(), h.Client, "nope", false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), server.ReasonLoginNotFound) {
		t.Errorf("err = %v, want %s", err, server.ReasonLoginNotFound)
	}
	if list, _ := h.Client.WebLogins(context.Background()); len(list.Logins) != 1 {
		t.Errorf("logins = %+v, want the one login kept", list.Logins)
	}
}

func TestWebLoginsAndRevoke_NoServerIsServerNotRunning(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "gxn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	c := apiclient.New(filepath.Join(dir, "none.sock"))
	for name, run := range map[string]func() error{
		"web-logins": func() error { return runWebLogins(context.Background(), c, false, &bytes.Buffer{}) },
		"web-revoke": func() error { return runWebRevoke(context.Background(), c, "x", false, &bytes.Buffer{}) },
	} {
		if err := run(); err == nil || !strings.Contains(err.Error(), server.ReasonServerNotRunning) {
			t.Errorf("%s: err = %v, want %s", name, err, server.ReasonServerNotRunning)
		}
	}
}

func TestWebLoginsAndRevoke_RefuseWhenTheWebUIIsOff(t *testing.T) {
	h := servertest.Start(t)
	for name, run := range map[string]func() error{
		"web-logins": func() error { return runWebLogins(context.Background(), h.Client, false, &bytes.Buffer{}) },
		"web-revoke": func() error { return runWebRevoke(context.Background(), h.Client, "x", false, &bytes.Buffer{}) },
	} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "the web UI is off") {
			t.Errorf("%s: err = %v, want the web UI is off", name, err)
		}
	}
}
