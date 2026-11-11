package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// pasteState tracks a bracketed paste across the keys that make it up.
type pasteState struct {
	pasting bool
	buf     []byte
	// escSeen holds an Esc and the bytes after it that still match pasteStart.
	// The Esc has already acted as an interrupt, which clears the input, so
	// snapText and snapImages restore it once the sequence proves a paste.
	escSeen    []byte
	snapText   string
	snapImages []string
}

// pasteKey takes one key and reports whether a bracketed paste consumed it. An
// Esc is not consumed: it interrupts at once, and the paste start that may
// follow it undoes the interrupt's effect on the input.
func (t *tui) pasteKey(key byte) bool {
	if t.opts.mouse {
		return false
	}
	p := &t.paste
	if p.pasting {
		p.buf = append(p.buf, key)
		if body, ok := bytes.CutSuffix(p.buf, []byte(PasteEnd)); ok {
			p.pasting, p.buf = false, nil
			t.pasted(string(body))
		}
		return true
	}
	if len(p.escSeen) > 0 {
		cand := append(p.escSeen, key)
		if strings.HasPrefix(PasteStart, string(cand)) {
			if string(cand) != PasteStart {
				p.escSeen = cand
				return true
			}
			p.escSeen, p.pasting = nil, true
			t.typed.Reset()
			t.typed.WriteString(p.snapText)
			t.images = p.snapImages
			return true
		}
		// Not a paste: what followed the Esc is typed text.
		t.typed.Write(p.escSeen[1:])
		p.escSeen = nil
	}
	if key == EscKey {
		p.escSeen = []byte{key}
		p.snapText, p.snapImages = t.typed.String(), t.images
	}
	return false
}

// pasted applies a finished paste: a lone path to an existing image adds a
// chip, anything else is text.
func (t *tui) pasted(body string) {
	path := strings.TrimSpace(body)
	if !IsImageFile(path) {
		t.typed.WriteString(body)
		t.draw()
		return
	}
	if t.typed.Len() == 0 {
		t.images = nil
	}
	t.images = append(t.images, path)
	fmt.Fprintf(&t.typed, "[Image #%d] ", len(t.images))
	t.draw()
}

// writeImagePrompt appends the two lines claude writes for a prompt with pasted
// images: the user's text, chips and image blocks, then a meta line naming where
// each image came from.
func writeImagePrompt(path, text string, images []string) error {
	promptID := fmt.Sprintf("p%d", time.Now().UnixNano())
	blocks := []map[string]any{{"type": "text", "text": text}}
	sources := []map[string]any{}
	for _, img := range images {
		blocks = append(blocks, map[string]any{"type": "image", "source": map[string]any{
			"type": "base64", "media_type": ImageMediaType(img), "data": "AA==",
		}})
		sources = append(sources, map[string]any{"type": "text", "text": "[Image: source: " + img + "]"})
	}
	if err := AppendEntry(path, "user", blocks, map[string]any{"promptId": promptID}); err != nil {
		return err
	}
	return AppendEntry(path, "user", sources, map[string]any{"isMeta": true, "promptId": promptID, "uuid": "fake-meta-" + promptID})
}
