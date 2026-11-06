package claudedoctor

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/transcript"
)

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYGD4DwABBAEAwS2OUAAAAABJRU5ErkJggg=="

// imageChip is the chip claude shows in the input box for the first pasted image.
const imageChip = "[Image #1]"

// writeTinyPNG saves a 1x1 PNG as dir/images/img-<n>-d0c70000.png, the plain
// ASCII naming gx gives Prompt images; dir itself may hold a space.
func writeTinyPNG(dir string, n int) (string, error) {
	data, err := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if err != nil {
		return "", err
	}
	imgs := filepath.Join(dir, "images")
	if err := os.MkdirAll(imgs, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(imgs, transcript.SavedImageName(n, "d0c70000", ".png"))
	return path, os.WriteFile(path, data, 0o644)
}

func waitScreen(h *holder.HolderClient, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		rows, _, err := h.Screen(0)
		if err == nil && strings.Contains(strings.Join(rows, "\n"), want) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%q not on screen after %s", want, timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// PtyImagePaste pastes a tiny PNG saved under dataDir into a live claude,
// waits for its chip, submits, and returns the screen frames plus the
// transcript lines claude wrote for the prompt.
func PtyImagePaste(p *nativerunner.Pty, cwd, dataDir string) (Recording, error) {
	var events []json.RawMessage
	rec, err := PtyLive(p, cwd, func(s agentrunner.Session) error {
		h, err := holder.DialHolder(filepath.Join(p.Root, PtyLiveLabel))
		if err != nil {
			return err
		}
		path, err := writeTinyPNG(dataDir, 1)
		if err != nil {
			return err
		}
		// TypeTextBy waits out the paste settle; an Enter sent straight after
		// the paste lands as a newline in the box instead of submitting.
		if err := h.TypeTextBy(path, true); err != nil {
			return err
		}
		if err := waitScreen(h, imageChip, 15*time.Second); err != nil {
			return err
		}
		if err := h.SendKeys("\r"); err != nil {
			return err
		}
		events, err = waitImageTranscript(p, s.SessionID, 30*time.Second)
		return err
	})
	rec.Events = events
	return rec, err
}

// waitImageTranscript polls the session's transcript until it holds an image
// block, returning the lines read so far if none shows in time.
func waitImageTranscript(p *nativerunner.Pty, sessionID string, timeout time.Duration) ([]json.RawMessage, error) {
	projects, err := harness.ClaudeProjectsDirIn(explicitEnv(p.Env))
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		var events []json.RawMessage
		if matches, _ := filepath.Glob(filepath.Join(projects, "*", sessionID+".jsonl")); len(matches) > 0 {
			if data, err := os.ReadFile(matches[0]); err == nil {
				for line := range bytes.SplitSeq(data, []byte("\n")) {
					if line = bytes.TrimSpace(line); len(line) > 0 && json.Valid(line) {
						events = append(events, json.RawMessage(bytes.Clone(line)))
					}
				}
			}
		}
		if _, ok := imageBlockMedia(events); ok || time.Now().After(deadline) {
			return events, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// PtyImageSlash pastes two images and then types a "/" command, leaving the
// send unblocked, and returns the frames. What claude does with the chips is
// recorded by checkImageSlash, never judged.
func PtyImageSlash(p *nativerunner.Pty, cwd, dataDir string) (Recording, error) {
	return PtyLive(p, cwd, func(s agentrunner.Session) error {
		h, err := holder.DialHolder(filepath.Join(p.Root, PtyLiveLabel))
		if err != nil {
			return err
		}
		for n := 1; n <= 2; n++ {
			path, err := writeTinyPNG(dataDir, n)
			if err != nil {
				return err
			}
			if err := h.Paste(path); err != nil {
				return err
			}
			if err := waitScreen(h, fmt.Sprintf("[Image #%d]", n), 15*time.Second); err != nil {
				return err
			}
		}
		if err := h.SendKeys("/model"); err != nil {
			return err
		}
		return waitScreen(h, "/model", 15*time.Second)
	})
}

// imageBlockMedia returns the media type of the first image block in a user
// line of the transcript.
func imageBlockMedia(events []json.RawMessage) (string, bool) {
	for _, e := range events {
		var line struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(e, &line) != nil || line.Type != "user" {
			continue
		}
		var blocks []struct {
			Type   string `json:"type"`
			Source struct {
				MediaType string `json:"media_type"`
			} `json:"source"`
		}
		if json.Unmarshal(line.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "image" {
				return b.Source.MediaType, true
			}
		}
	}
	return "", false
}

// checkImagePaste catches a claude update that stops treating a pasted image
// path as an attachment: the chip must show on screen and the transcript must
// hold the image block.
func checkImagePaste(in Input) (string, bool) {
	chip := false
	for _, f := range in.Frames {
		if strings.Contains(f, imageChip) {
			chip = true
			break
		}
	}
	if !chip {
		return fmt.Sprintf("no %s chip in %d frames; the pasted path is no longer attached", imageChip, len(in.Frames)), false
	}
	media, ok := imageBlockMedia(in.Events)
	if !ok {
		return fmt.Sprintf("chip shown but no image block in %d transcript lines", len(in.Events)), false
	}
	if media != "image/png" {
		return fmt.Sprintf("image block has media type %q, want image/png", media), false
	}
	return "chip shown and transcript holds an image/png block", true
}

// checkImageSlash records what claude did with two chips once a "/" command
// was typed. It never fails: the outcome is for a person to read.
func checkImageSlash(in Input) (string, bool) {
	if len(in.Frames) == 0 {
		return "recorded no frames", true
	}
	last := in.Frames[len(in.Frames)-1]
	return fmt.Sprintf("after the / command: chip 1 %s, chip 2 %s",
		present(last, "[Image #1]"), present(last, "[Image #2]")), true
}

func present(frame, chip string) string {
	if strings.Contains(frame, chip) {
		return "kept"
	}
	return "gone"
}
