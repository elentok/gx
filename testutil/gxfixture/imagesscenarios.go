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
	"github.com/elentok/gx/server/designs/drafts"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/spec"
	"github.com/elentok/gx/transcript"
)

// agentsImagesScenario is the agents scenario with saved images in the chat of
// its first Claude session and its Codex session.
func agentsImagesScenario() scenario {
	sc := spec.AgentsScenario()
	sc.Sessions = slices.Clone(sc.Sessions)
	for i := range sc.Sessions {
		if i == 0 || sc.Sessions[i].Harness == "codex" {
			sc.Sessions[i].Images = true
		}
	}
	return sc
}

// pngBase64 is a 1x1 PNG.
const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg=="

// seedEpicImages saves n PNGs into a map's images folder; none makes no folder.
func seedEpicImages(dir string, n int) error {
	if n == 0 {
		return nil
	}
	png, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i := 1; i <= n; i++ {
		if err := os.WriteFile(filepath.Join(dir, transcript.SavedImageName(i, fmt.Sprintf("%08x", i), ".png")), png, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// seedDrafts stores the drafts with the real draft store, so the server reads
// them as it reads a draft a person saved.
func seedDrafts(stateDir string, list []spec.Draft) error {
	png, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		return err
	}
	store := drafts.New(stateDir)
	for _, d := range list {
		saved, err := store.Create(d.Project, d.Slug, d.Idea, d.Name)
		if err != nil {
			return err
		}
		for range d.Images {
			if res, err := store.AddImage(saved.ID, png); err != nil || res.Refused {
				return fmt.Errorf("draft %q: image: %v %+v", d.Name, err, res)
			}
		}
	}
	return nil
}

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
	if err := f.appendChatLines(id, harness, lines); err != nil {
		return err
	}
	return f.waitFor(id, "the image messages", func(_ wire.SessionInfo, c wire.SessionChatResponse) bool {
		return slices.ContainsFunc(c.Messages, func(msg wire.SessionChatMessage) bool { return len(msg.Images) == 10 })
	})
}

// seedInterrupted appends the lines of one interrupted turn, as each harness
// writes them, and waits until the chat shows the separator.
func (f sessionSeeder) seedInterrupted(id string, harness string) error {
	var lines []string
	switch harness {
	case "codex":
		lines = []string{
			`{"timestamp":"2026-01-01T10:00:00Z","type":"event_msg","payload":{"type":"turn_aborted","reason":"interrupted"}}`,
			`{"timestamp":"2026-01-01T10:00:00Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<turn_aborted>\nThe user interrupted the previous turn on purpose.\n</turn_aborted>"}]}}`,
		}
	default:
		lines = []string{`{"type":"user","uuid":"u-interrupted","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"` + transcript.InterruptedByUserText + `"}]}}`}
	}
	if err := f.appendChatLines(id, harness, lines); err != nil {
		return err
	}
	return f.waitFor(id, "the interrupted message", func(_ wire.SessionInfo, c wire.SessionChatResponse) bool {
		return slices.ContainsFunc(c.Messages, func(msg wire.SessionChatMessage) bool { return msg.Kind == transcript.ChatKindInterrupted })
	})
}

// appendChatLines appends transcript lines to the session's Claude transcript or
// Codex rollout, once the harness has reported its conversation id.
func (f sessionSeeder) appendChatLines(id, harness string, lines []string) error {
	dir := filepath.Join(f.root, "data", "gx", "sessions", id)
	var meta sessions.Meta
	var path string
	var err error
	// The conversation id can change after the first report, so the meta is read
	// again on each try, and the Codex rollout file may lag the id.
	for deadline := time.Now().Add(sessionWaitTimeout); ; time.Sleep(100 * time.Millisecond) {
		if m, merr := sessions.ReadMeta(dir); merr == nil {
			meta = m
		}
		if meta.ConversationID != "" {
			if harness != "codex" {
				path = transcript.PathIn(filepath.Join(f.root, "home"), meta.Dir, meta.ConversationID)
				break
			}
			var ok bool
			if path, ok, err = codexsession.RolloutPathIn(filepath.Join(f.root, "home", ".codex"), meta.ConversationID); err == nil && ok {
				break
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("session %s: no chat file (conversation %q): %v", id, meta.ConversationID, err)
		}
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
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
