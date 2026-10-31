package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

var archivedRollout = []string{
	`{"timestamp":"2026-01-01T10:00:00Z","ordinal":0,"type":"session_meta","payload":{"cli_version":"0.166.0"}}`,
	`{"timestamp":"2026-01-01T10:00:01Z","ordinal":1,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`,
	`{"timestamp":"2026-01-01T10:00:03Z","ordinal":3,"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}}`,
}

// writeZstd compresses lines into the .jsonl.zst that replaces plainPath.
func writeZstd(t *testing.T, plainPath string, lines ...string) string {
	t.Helper()
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	path := plainPath + ".zst"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, enc.EncodeAll([]byte(strings.Join(lines, "\n")+"\n"), nil), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSessionChat_ArchivedRolloutIsFlaggedAndNotDecoded(t *testing.T) {
	t.Parallel()
	h, ctx, id, path := startCodexChatSession(t)
	// Not valid zstd: a plain read that opened the archive would fail.
	archive := path + ".zst"
	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("not zstd"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := h.Client.SessionChat(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Archived || len(got.Messages) != 0 {
		t.Errorf("chat = %+v, want archived with no messages", got)
	}
	if _, err := h.Client.SessionChatArchive(ctx, id); err == nil || !strings.Contains(err.Error(), "422") {
		t.Errorf("corrupt archive: err = %v, want a 422", err)
	}
}

func TestSessionChat_ArchiveRouteMatchesThePlainRollout(t *testing.T) {
	t.Parallel()
	h, ctx, id, path := startCodexChatSession(t)
	writeTranscript(t, path, archivedRollout...)
	plain, err := h.Client.SessionChat(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	writeZstd(t, path, archivedRollout...)
	got, err := h.Client.SessionChatArchive(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.Messages) != 2 || len(got.Messages) != len(plain.Messages) {
		t.Fatalf("archive messages = %+v, plain = %+v", got.Messages, plain.Messages)
	}
	for i := range plain.Messages {
		if got.Messages[i] != plain.Messages[i] {
			t.Errorf("message %d = %+v, want %+v", i, got.Messages[i], plain.Messages[i])
		}
	}
}

func TestSessionChat_ArchiveRouteWithoutAnArchiveIs404(t *testing.T) {
	t.Parallel()
	h, ctx, id, _ := startCodexChatSession(t)
	if _, err := h.Client.SessionChatArchive(ctx, id); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want a 404", err)
	}
}
