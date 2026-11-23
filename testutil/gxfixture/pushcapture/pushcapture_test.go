package pushcapture

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServer_ControlFilePicksTheAnswerAndEveryPostIsRecorded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, control := filepath.Join(dir, "capture.jsonl"), filepath.Join(dir, "control")
	s, err := Start(path, control)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	post := func() int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, s.URL()+"/push/1", strings.NewReader("secret-body"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("TTL", "60")
		req.Header.Set("Topic", "tag")
		req.Header.Set("Authorization", "vapid t=x")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(); got != http.StatusCreated {
		t.Errorf("no control file: status %d, want 201", got)
	}
	if err := os.WriteFile(control, []byte("500\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := post(); got != http.StatusInternalServerError {
		t.Errorf("control 500: status %d, want 500", got)
	}
	if err := os.WriteFile(control, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := post(); got != http.StatusCreated {
		t.Errorf("empty control file: status %d, want 201", got)
	}

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
	if len(got) != 3 {
		t.Fatalf("captured %d lines, want 3: %+v", len(got), got)
	}
	l := got[0]
	if l.Path != "/push/1" || l.TTL != "60" || l.Topic != "tag" || !l.Authorization || l.BodyLength != len("secret-body") {
		t.Errorf("line = %+v", l)
	}
	if raw, _ := os.ReadFile(path); strings.Contains(string(raw), "secret-body") {
		t.Error("the capture file holds the body")
	}
}
