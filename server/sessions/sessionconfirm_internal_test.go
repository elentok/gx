package sessions

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFirstReplyReaders(t *testing.T) {
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	claude := func(ts, errKind string) string {
		extra := ""
		if errKind != "" {
			extra = `,"isApiErrorMessage":true,"error":"` + errKind + `"`
		}
		return `{"type":"assistant","uuid":"a","timestamp":"` + ts + `","message":{"content":[{"type":"text","text":"x"}]}` + extra + "}\n"
	}
	codex := func(ts, errKind string) string {
		errPart := ""
		if errKind != "" {
			errPart = `,"error":{"message":"m","codex_error_info":"` + errKind + `"}`
		}
		return `{"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"task_complete"` + errPart + "}}\n"
	}
	const old, fresh = "2026-01-01T09:00:00Z", "2026-01-01T10:00:01Z"
	cases := []struct {
		name         string
		reader       firstReplyReader
		content      string
		wantNotFound bool
		wantFound    bool
	}{
		{"claude: no reply after the switch", claudeReplyReader{}, claude(old, "model_not_found"), false, false},
		{"claude: model_not_found", claudeReplyReader{}, claude(fresh, "model_not_found"), true, true},
		{"claude: clean reply", claudeReplyReader{}, claude(fresh, ""), false, true},
		{"claude: other error", claudeReplyReader{}, claude(fresh, "rate_limit"), false, true},
		{"codex: no reply after the switch", codexReplyReader{}, codex(old, "model_not_found"), false, false},
		{"codex: model_not_found", codexReplyReader{}, codex(fresh, "model_not_found"), true, true},
		{"codex: clean reply", codexReplyReader{}, codex(fresh, ""), false, true},
		{"codex: other error", codexReplyReader{}, codex(fresh, "usage_limit_exceeded"), false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "log.jsonl")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			notFound, found, err := tc.reader.FirstReply(path, at)
			if err != nil || notFound != tc.wantNotFound || found != tc.wantFound {
				t.Errorf("FirstReply = notFound %v, found %v, err %v; want %v, %v", notFound, found, err, tc.wantNotFound, tc.wantFound)
			}
		})
	}
}
