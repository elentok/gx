package chat

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elentok/gx/server/wire"
)

// savedImage writes a saved Prompt image into the session dir's images folder.
func savedImage(t *testing.T, sessionDir, name string) string {
	t.Helper()
	dir := filepath.Join(sessionDir, ImagesDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func claudeImageLines(promptID, text, source string) []string {
	return []string{
		`{"type":"user","uuid":"u-` + promptID + `","promptId":"` + promptID + `","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"` + text + `"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}}`,
		`{"type":"user","isMeta":true,"promptId":"` + promptID + `","message":{"content":[{"type":"text","text":"[Image: source: ` + source + `]"}]}}`,
	}
}

func TestRead_ClaudeListsImagesAndPlaceholders(t *testing.T) {
	t.Parallel()
	f := newClaudeFixture(t)
	f.src.dir = t.TempDir()
	saved := savedImage(t, f.src.dir, "img-1-aa11.png")
	outside := savedImage(t, t.TempDir(), "img-1-bb22.png")
	lines := append(claudeImageLines("p1", "[Image #1] look", saved), claudeImageLines("p2", "[Image #1]", outside)...)
	// A compaction after the images must not hide them.
	lines = append(lines, `{"type":"system","uuid":"c1","subtype":"compact_boundary","timestamp":"2026-01-01T10:00:08Z"}`)
	writeLines(t, f.path, lines...)
	got := f.chat(t, "/v1/sessions/chat")
	if len(got.Messages) != 3 {
		t.Fatalf("messages = %+v", got.Messages)
	}
	if want := []wire.SessionChatImage{{ID: "aa11"}}; !reflect.DeepEqual(got.Messages[0].Images, want) {
		t.Errorf("saved image = %+v, want %+v", got.Messages[0].Images, want)
	}
	if m := got.Messages[1]; m.Text != "" || !reflect.DeepEqual(m.Images, []wire.SessionChatImage{{Placeholder: true}}) {
		t.Errorf("images-only outside-folder message = %+v, want empty text and one placeholder", m)
	}
}

func TestRead_CodexListsImages(t *testing.T) {
	t.Parallel()
	f := newCodexFixture(t)
	f.src.dir = t.TempDir()
	saved := savedImage(t, f.src.dir, "img-1-cc33.png")
	writeLines(t, f.path,
		`{"timestamp":"2026-01-01T10:00:00Z","ordinal":0,"type":"session_meta","payload":{"cli_version":"0.166.0"}}`,
		`{"timestamp":"2026-01-01T10:00:01Z","ordinal":1,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<image name=[Image #1] path=\"`+saved+`\">"},{"type":"input_image","image_url":"data:image/png;base64,AAAA"},{"type":"input_text","text":"</image>"}]}}`,
	)
	got := f.chat(t, "/v1/sessions/chat")
	if len(got.Messages) != 1 || got.Messages[0].Text != "" || !reflect.DeepEqual(got.Messages[0].Images, []wire.SessionChatImage{{ID: "cc33"}}) {
		t.Errorf("messages = %+v, want one images-only message with cc33", got.Messages)
	}
}
