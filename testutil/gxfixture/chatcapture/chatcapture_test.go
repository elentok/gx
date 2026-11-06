package chatcapture

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServer_SlackAndTelegramPostsBecomeTwoJSONLines(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "capture.jsonl")
	s, err := Start(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	post := func(url, body string) {
		t.Helper()
		resp, err := http.Post(url, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST %s = %d, want 200", url, resp.StatusCode)
		}
	}
	post(s.SlackURL(), `{"text":"hello"}`)
	post(s.TelegramBaseURL()+"/bot123:abc/sendMessage", `{"chat_id":"42","text":"*hi*","parse_mode":"MarkdownV2"}`)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var got []Line
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l Line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatal(err)
		}
		got = append(got, l)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("captured %d lines, want 2: %+v", len(got), got)
	}
	if got[0].Transport != "slack" || got[0].Text != "hello" || got[0].ParseMode != "" || got[0].At.IsZero() {
		t.Errorf("slack line = %+v", got[0])
	}
	if got[1].Transport != "telegram" || got[1].Text != "*hi*" || got[1].ParseMode != "MarkdownV2" {
		t.Errorf("telegram line = %+v", got[1])
	}
}
