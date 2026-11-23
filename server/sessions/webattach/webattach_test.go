package webattach_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

func post(t *testing.T, f rigs.PromptFixture, path string, req wire.SessionAttachRequest) wire.SessionResult {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	code, out := servertest.WebPostJSON(t, f.Base+path, string(body), f.Ck, "", servertest.JSONPost(f.Base))
	if code != http.StatusOK {
		t.Fatalf("POST %s = %d %s", path, code, out)
	}
	var res wire.SessionResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func typeKey(t *testing.T, f rigs.PromptFixture, client string, r rune) wire.SessionResult {
	t.Helper()
	return post(t, f, "/v1/sessions/attach/input", wire.SessionAttachRequest{
		ID: f.ID, Client: client, Key: &wire.AttachKey{Code: int(r), Text: string(r)},
	})
}

// events opens a client's event stream and returns the lines it delivers.
func events(t *testing.T, f rigs.PromptFixture, client string) <-chan string {
	t.Helper()
	u := f.Base + "/v1/sessions/attach/events/?id=" + url.QueryEscape(f.ID) + "&client=" + url.QueryEscape(client)
	req, err := http.NewRequestWithContext(f.Ctx, http.MethodGet, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(f.Ck)
	resp, err := servertest.WebHTTPClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events = %d", resp.StatusCode)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	lines := make(chan string, 16)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		// A read error is the stream ending, which waitLine reports.
		_ = sc.Err()
	}()
	return lines
}

func waitLine(t *testing.T, lines <-chan string, want string) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("event stream ended before %q", want)
			}
			if l == want {
				return
			}
		case <-timeout:
			t.Fatalf("no %q on the event stream", want)
		}
	}
}

func TestWebAttach_TwoClientsHoldSeparateAttaches(t *testing.T) {
	t.Parallel()
	f := rigs.StartCodexPromptSession(t)
	c, err := holder.DialHolder(filepath.Join(f.H.SessionsDir, f.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range "zq1" {
		if res := typeKey(t, f, "tab-a", r); res.Refused {
			t.Fatalf("tab-a key refused: %+v", res)
		}
	}
	servertest.WaitForScreen(t, c, "zq1")
	// The holder lets one attach own the input, so tab-b being blocked while
	// tab-a types is what shows it is a second attach, not tab-a's again.
	for _, r := range "yw2" {
		if res := typeKey(t, f, "tab-b", r); res.Refused {
			t.Fatalf("tab-b key refused: %+v", res)
		}
	}
	typeKey(t, f, "tab-a", '!')
	servertest.WaitForScreen(t, c, "zq1!")
	if servertest.ScreenHas(t, c, "yw2") {
		t.Error("keys from a client that does not own the input reached the child")
	}
}

func TestWebAttach_DetachEndsTheClientsStream(t *testing.T) {
	t.Parallel()
	f := rigs.StartCodexPromptSession(t)
	lines := events(t, f, "tab-a")
	other := events(t, f, "tab-b")
	post(t, f, "/v1/sessions/attach/detach", wire.SessionAttachRequest{ID: f.ID, Client: "tab-a"})
	waitLine(t, lines, "event: closed")
	select {
	case l, ok := <-other:
		if !ok || l == "event: closed" {
			t.Errorf("detaching tab-a ended tab-b's stream: %q", l)
		}
	default:
	}
}

func TestWebAttach_IdleAttachCloses(t *testing.T) {
	t.Parallel()
	f := rigs.StartCodexPromptSessionWith(t, func(c *server.Config) { c.WebAttachIdle = 200 * time.Millisecond })
	waitLine(t, events(t, f, "tab-a"), "event: closed")
}

func TestWebAttach_Refusals(t *testing.T) {
	t.Parallel()
	f := rigs.StartCodexPromptSession(t)
	cases := map[string]wire.SessionAttachRequest{
		"unknown session": {ID: "nope", Client: "tab-a", Key: &wire.AttachKey{Code: 'a', Text: "a"}},
		"no client":       {ID: f.ID, Key: &wire.AttachKey{Code: 'a', Text: "a"}},
		"no input":        {ID: f.ID, Client: "tab-a"},
		"two inputs":      {ID: f.ID, Client: "tab-a", Paste: "x", Key: &wire.AttachKey{Code: 'a'}},
		"long client":     {ID: f.ID, Client: strings.Repeat("c", 65), Paste: "x"},
	}
	for name, req := range cases {
		if res := post(t, f, "/v1/sessions/attach/input", req); !res.Refused {
			t.Errorf("%s: not refused: %+v", name, res)
		}
	}
}
