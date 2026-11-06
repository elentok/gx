package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// agentsImagesScenario is the agents scenario with saved images in the chat of
// its first Claude session and its Codex session.
func agentsImagesScenario() scenario {
	sc := agentsScenario()
	sc.sessions = slices.Clone(sc.sessions)
	for i := range sc.sessions {
		if i == 0 || sc.sessions[i].harness == "codex" {
			sc.sessions[i].images = true
		}
	}
	return sc
}

// pngBase64 is a 1x1 PNG.
const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg=="

// imageMessage is one user message of the seeded chat: its text, how many saved
// images it carries, and whether its source path lies outside the session folder.
type imageMessage struct {
	text    string
	images  int
	outside bool
}

var seededImageMessages = []imageMessage{
	{text: "look at these", images: 3},
	{images: 2},
	{text: "no image here"},
	{text: "moved away", images: 1, outside: true},
	{text: "the whole set", images: 10},
}

// seedImages saves the images into the session's folder and appends the
// transcript lines that reference them.
func (f sessionSeeder) seedImages(id string, harness string) error {
	dir := filepath.Join(f.root, "data", "gx", "sessions", id)
	// The conversation id lands in the meta when the harness reports it.
	var meta sessions.Meta
	var err error
	for deadline := time.Now().Add(sessionWaitTimeout); ; time.Sleep(100 * time.Millisecond) {
		if meta, err = sessions.ReadMeta(dir); err == nil && meta.ConversationID != "" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %s: no conversation id (%v)", id, err)
		}
	}
	png, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		return err
	}
	imagesDir := filepath.Join(dir, chat.ImagesDir)
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return err
	}
	saved := 0
	var lines []string
	for n, m := range seededImageMessages {
		paths := make([]string, m.images)
		for i := range paths {
			folder := imagesDir
			if m.outside {
				folder = filepath.Join(f.root, "elsewhere")
				if err := os.MkdirAll(folder, 0o755); err != nil {
					return err
				}
			} else {
				saved++
			}
			paths[i] = filepath.Join(folder, transcript.SavedImageName(saved, fmt.Sprintf("%08x", n*100+i+1), ".png"))
			if err := os.WriteFile(paths[i], png, 0o644); err != nil {
				return err
			}
		}
		if harness == "codex" {
			lines = append(lines, codexImageLine(m, paths))
		} else {
			lines = append(lines, claudeImageLines(n, m, paths)...)
		}
	}
	var path string
	if harness == "codex" {
		var ok bool
		if path, ok, err = codexsession.RolloutPathIn(filepath.Join(f.root, "home", ".codex"), meta.ConversationID); err != nil || !ok {
			return fmt.Errorf("session %s: no rollout (found %v): %v", id, ok, err)
		}
	} else {
		path = transcript.PathIn(filepath.Join(f.root, "home"), meta.Dir, meta.ConversationID)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return f.waitFor(id, "the image messages", func(_ wire.SessionInfo, c wire.SessionChatResponse) bool {
		return slices.ContainsFunc(c.Messages, func(msg wire.SessionChatMessage) bool { return len(msg.Images) == 10 })
	})
}

// chips is "[Image #1] [Image #2] ..." followed by the text.
func chips(m imageMessage) string {
	parts := make([]string, 0, m.images+1)
	for i := 1; i <= m.images; i++ {
		parts = append(parts, fmt.Sprintf("[Image #%d]", i))
	}
	if m.text != "" {
		parts = append(parts, m.text)
	}
	return strings.Join(parts, " ")
}

// claudeImageLines is the person line, then the isMeta line naming the saved
// files, as Claude writes a pasted image.
func claudeImageLines(n int, m imageMessage, paths []string) []string {
	promptID := fmt.Sprintf("img-prompt-%d", n)
	blocks := []string{fmt.Sprintf(`{"type":"text","text":%q}`, chips(m))}
	var sources []string
	for _, p := range paths {
		blocks = append(blocks, `{"type":"image","source":{"type":"base64","media_type":"image/png","data":"`+pngBase64+`"}}`)
		sources = append(sources, fmt.Sprintf(`{"type":"text","text":%q}`, "[Image: source: "+p+"]"))
	}
	lines := []string{fmt.Sprintf(`{"type":"user","uuid":"u-%s","promptId":%q,"timestamp":"2026-01-01T10:00:%02dZ","message":{"content":[%s]}}`,
		promptID, promptID, n, strings.Join(blocks, ","))}
	if len(paths) > 0 {
		lines = append(lines, fmt.Sprintf(`{"type":"user","isMeta":true,"promptId":%q,"message":{"content":[%s]}}`, promptID, strings.Join(sources, ",")))
	}
	return lines
}

// codexImageLine is one user response_item, each image wrapped as Codex does.
func codexImageLine(m imageMessage, paths []string) string {
	var parts []string
	for i, p := range paths {
		parts = append(parts,
			fmt.Sprintf(`{"type":"input_text","text":%q}`, fmt.Sprintf(`<image name=[Image #%d] path="%s">`, i+1, p)),
			`{"type":"input_image","image_url":"data:image/png;base64,`+pngBase64+`"}`,
			`{"type":"input_text","text":"</image>"}`)
	}
	if m.text != "" {
		parts = append(parts, fmt.Sprintf(`{"type":"input_text","text":%q}`, m.text))
	}
	return fmt.Sprintf(`{"timestamp":"2026-01-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[%s]}}`, strings.Join(parts, ","))
}
