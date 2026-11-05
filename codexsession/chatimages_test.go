package codexsession

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/elentok/gx/transcript"
)

func imageUserLine(t *testing.T, items ...map[string]string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type":    "response_item",
		"payload": map[string]any{"type": "message", "role": "user", "content": items},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func imageWrapped(path, data string, n int) []map[string]string {
	chip := "[Image #" + string(rune('0'+n)) + "]"
	return []map[string]string{
		{"type": "input_text", "text": `<image name=` + chip + ` path="` + path + `">`},
		{"type": "input_image", "image_url": data},
		{"type": "input_text", "text": "</image>"},
	}
}

func text(s string) map[string]string { return map[string]string{"type": "input_text", "text": s} }

func savedPNG(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readImages(t *testing.T, dir string, lines ...string) []transcript.ChatMessage {
	t.Helper()
	chat, err := ReadChat(strings.NewReader(strings.Join(lines, "\n")+"\n"), transcript.WithImagesDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	return chat.Messages
}

func TestReadChat_ImageWrappersBecomeImages(t *testing.T) {
	dir := t.TempDir()
	a := savedPNG(t, dir, "img-1-aa11.png")
	b := savedPNG(t, dir, "img-2-bb22.png")
	items := append(imageWrapped(a, "data:image/png;base64,AAAA", 1), imageWrapped(b, "data:image/png;base64,BBBB", 2)...)
	items = append(items, text("[Image #1] [Image #2] what are these"))

	msgs := readImages(t, dir, imageUserLine(t, items...))

	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	if msgs[0].Text != "what are these" {
		t.Errorf("text = %q", msgs[0].Text)
	}
	want := []transcript.ChatImage{{ID: "aa11"}, {ID: "bb22"}}
	if !reflect.DeepEqual(msgs[0].Images, want) {
		t.Errorf("images = %+v, want %+v", msgs[0].Images, want)
	}
}

func TestReadChat_ImagesOnlyMessageIsKept(t *testing.T) {
	dir := t.TempDir()
	a := savedPNG(t, dir, "img-1-aa11.png")

	msgs := readImages(t, dir, imageUserLine(t, imageWrapped(a, "data:x", 1)...))

	if len(msgs) != 1 || msgs[0].Text != "" || len(msgs[0].Images) != 1 || msgs[0].Role != transcript.ChatRolePerson {
		t.Fatalf("msgs = %+v", msgs)
	}
}

func TestReadChat_OutsideFolderAndMissingFileArePlaceholders(t *testing.T) {
	dir := t.TempDir()
	outside := savedPNG(t, t.TempDir(), "img-1-aa11.png")
	items := append(imageWrapped(outside, "data:x", 1), imageWrapped(filepath.Join(dir, "img-2-bb22.png"), "data:x", 2)...)

	msgs := readImages(t, dir, imageUserLine(t, items...))

	want := []transcript.ChatImage{{Placeholder: true}, {Placeholder: true}}
	if len(msgs) != 1 || !reflect.DeepEqual(msgs[0].Images, want) {
		t.Fatalf("msgs = %+v", msgs)
	}
}

func TestReadChat_LineOverTenMBDoesNotBreakTheRead(t *testing.T) {
	dir := t.TempDir()
	a := savedPNG(t, dir, "img-1-aa11.png")
	big := imageUserLine(t, append(imageWrapped(a, strings.Repeat("A", 11<<20), 1), text("[Image #1] look"))...)

	msgs := readImages(t, dir, big, imageUserLine(t, text("and then")))

	if len(msgs) != 2 || msgs[0].Text != "look" || msgs[1].Text != "and then" {
		t.Fatalf("msgs = %+v", msgs)
	}
}

func TestReadChat_CompactionThenImageMessageAndInjectedStaysHidden(t *testing.T) {
	dir := t.TempDir()
	a := savedPNG(t, dir, "img-1-aa11.png")
	lines := []string{
		imageUserLine(t, text("# AGENTS.md instructions for /x")),
		`{"type":"compacted","payload":{}}`,
		imageUserLine(t, append(imageWrapped(a, "data:x", 1), text("[Image #1] hi"))...),
	}

	msgs := readImages(t, dir, lines...)

	if len(msgs) != 2 || msgs[0].Kind != transcript.ChatKindCompacted || msgs[1].Text != "hi" || len(msgs[1].Images) != 1 {
		t.Fatalf("msgs = %+v", msgs)
	}
}
