package transcript

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func imageUserLine(promptID, text string, nImages int) string {
	blocks := `{"type":"text","text":"` + text + `"}`
	for range nImages {
		blocks += `,{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}`
	}
	return `{"type":"user","uuid":"u-` + promptID + `","promptId":"` + promptID + `","message":{"content":[` + blocks + `]}}`
}

func imageMetaLine(promptID string, paths ...string) string {
	var blocks []string
	for _, p := range paths {
		blocks = append(blocks, `{"type":"text","text":"[Image: source: `+p+`]"}`)
	}
	return `{"type":"user","isMeta":true,"promptId":"` + promptID + `","message":{"content":[` + strings.Join(blocks, ",") + `]}}`
}

func readLines(t *testing.T, dir string, lines ...string) []ChatMessage {
	t.Helper()
	msgs, err := ReadChatFrom(strings.NewReader(strings.Join(lines, "\n")+"\n"), WithImagesDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	return msgs
}

func savedFile(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadChat_ImagesPairChipsWithSourcesInOrder(t *testing.T) {
	dir := t.TempDir()
	a := savedFile(t, dir, "img-1-aa11.png")
	b := savedFile(t, dir, "img-2-bb22.png")
	msgs := readLines(t, dir,
		imageUserLine("p1", "[Image #1] [Image #2] name the color", 2),
		imageMetaLine("p1", a, b),
	)
	if len(msgs) != 1 || msgs[0].Text != "name the color" || msgs[0].Role != ChatRolePerson {
		t.Fatalf("msgs = %+v", msgs)
	}
	if want := []ChatImage{{ID: "aa11"}, {ID: "bb22"}}; !reflect.DeepEqual(msgs[0].Images, want) {
		t.Fatalf("images = %+v, want %+v", msgs[0].Images, want)
	}
}

func TestReadChat_ImagesOnlyMessageIsKept(t *testing.T) {
	dir := t.TempDir()
	a := savedFile(t, dir, "img-1-aa11.png")
	msgs := readLines(t, dir, imageUserLine("p1", "[Image #1]", 1), imageMetaLine("p1", a))
	if len(msgs) != 1 || msgs[0].Text != "" || len(msgs[0].Images) != 1 || msgs[0].Images[0].ID != "aa11" {
		t.Fatalf("msgs = %+v", msgs)
	}
}

func TestReadChat_ImagePlaceholders(t *testing.T) {
	dir := t.TempDir()
	outside := savedFile(t, t.TempDir(), "img-1-aa11.png")
	msgs := readLines(t, dir,
		imageUserLine("p1", "[Image #1] [Image #2] look", 2),
		imageMetaLine("p1", outside, filepath.Join(dir, "img-2-bb22.png")),
	)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %+v", msgs)
	}
	if want := []ChatImage{{Placeholder: true}, {Placeholder: true}}; !reflect.DeepEqual(msgs[0].Images, want) {
		t.Fatalf("images = %+v, want %+v", msgs[0].Images, want)
	}
}

func TestReadChat_ImageWithoutSourceLineIsPlaceholder(t *testing.T) {
	msgs := readLines(t, t.TempDir(), imageUserLine("p1", "[Image #1] hi", 1))
	if len(msgs) != 1 || !reflect.DeepEqual(msgs[0].Images, []ChatImage{{Placeholder: true}}) {
		t.Fatalf("msgs = %+v", msgs)
	}
}

func TestReadChat_PlainUserLineHasNoImages(t *testing.T) {
	msgs := readLines(t, t.TempDir(), `{"type":"user","uuid":"u1","message":{"content":"hello [Image #1]"}}`)
	if len(msgs) != 1 || msgs[0].Images != nil || msgs[0].Text != "hello" {
		t.Fatalf("msgs = %+v", msgs)
	}
}

func TestReadChat_LineOverTenMegabytesKeepsLaterLines(t *testing.T) {
	big := `{"type":"user","uuid":"u1","message":{"content":[{"type":"text","text":"big"},{"type":"image","source":{"data":"` +
		strings.Repeat("A", 11<<20) + `"}}]}}`
	msgs := readLines(t, t.TempDir(), big, `{"type":"assistant","uuid":"a1","message":{"content":[{"type":"text","text":"after"}]}}`)
	if len(msgs) != 2 || msgs[1].Text != "after" {
		t.Fatalf("got %d msgs: %+v", len(msgs), msgs)
	}
}

func TestReadChat_ImageBeforeCompactBoundaryKeepsItsImages(t *testing.T) {
	dir := t.TempDir()
	a := savedFile(t, dir, "img-1-aa11.png")
	msgs := readLines(t, dir,
		imageUserLine("p1", "[Image #1] x", 1),
		imageMetaLine("p1", a),
		`{"type":"system","subtype":"`+compactBoundarySubtype+`","uuid":"c1"}`,
	)
	if len(msgs) != 2 || msgs[0].Images[0].ID != "aa11" || msgs[1].Kind != ChatKindCompacted {
		t.Fatalf("msgs = %+v", msgs)
	}
}
