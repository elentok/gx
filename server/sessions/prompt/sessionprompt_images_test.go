package prompt_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// pasted is one bracketed paste of path, as the fake program logs it.
func pasted(path string) string { return "\x1b[200~" + path + "\x1b[201~" }

// upload saves n images to the session and returns their ids and paths.
func (f promptFixture) upload(t *testing.T, n int) (ids, paths []string) {
	t.Helper()
	for range n {
		res := f.addImage(t, f.id, imageBytes(pngHead, 16))
		if res.Refused {
			t.Fatalf("upload = %+v", res)
		}
		ids = append(ids, res.ID)
		paths = append(paths, filepath.Join(f.h.SessionsDir, f.id, "images", res.Name))
	}
	return ids, paths
}

func (f promptFixture) promptImages(t *testing.T, text string, ids ...string) wire.SessionResult {
	t.Helper()
	res, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{ID: f.id, Text: text, Images: ids})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// wantEntry is the submitted-log line of a prompt: each path pasted, a space,
// the text.
func wantEntry(text string, paths ...string) string {
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString(pasted(p))
	}
	sb.WriteString(" " + text)
	return strconv.Quote(sb.String()) + "\n"
}

func (f promptFixture) deleteImage(t *testing.T, id string) wire.SessionImageResult {
	t.Helper()
	res, err := f.h.Client.Sessions.RemoveImage(f.ctx, wire.SessionImageRequest{ID: f.id, Image: id})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSessionPromptImages_PastesEachImageThenTheText(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	ids, paths := f.upload(t, 2)
	if res := f.promptImages(t, "look at these", ids...); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, wantEntry("look at these", paths...))
}

func TestSessionPromptImages_ImagesOnlySends(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	ids, paths := f.upload(t, 1)
	if res := f.promptImages(t, "", ids...); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, wantEntry("", paths...))
}

func TestSessionPromptImages_Refusals(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	other := startPromptSession(t)
	otherIDs, _ := other.upload(t, 1)
	eleven, _ := f.upload(t, 11)
	sent, sentPaths := f.upload(t, 1)
	if res := f.promptImages(t, "first", sent...); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, wantEntry("first", sentPaths...))
	for name, tc := range map[string]struct {
		text   string
		ids    []string
		reason string
	}{
		"neither text nor images": {"", nil, wire.ReasonEmptyPrompt},
		"eleven images":           {"x", eleven, wire.ReasonImageTooMany},
		"unknown id":              {"x", []string{"00000000"}, wire.ReasonImageNotFound},
		"another session's id":    {"x", otherIDs, wire.ReasonImageNotFound},
		"an already sent id":      {"x", sent, wire.ReasonImageSent},
	} {
		if res := f.promptImages(t, tc.text, tc.ids...); !res.Refused || res.Reason != tc.reason {
			t.Errorf("%s: prompt = %+v, want %s", name, res, tc.reason)
		}
	}
	fresh, _ := f.upload(t, 1)
	report(t, f.conv, agentfake.Control{State: "working"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	if res := f.promptImages(t, "x", fresh...); !res.Refused || res.Reason != wire.ReasonSessionNotIdle {
		t.Errorf("busy session: prompt = %+v, want %s", res, wire.ReasonSessionNotIdle)
	}
}

func TestSessionPromptImages_TextCapCountsOnlyTheText(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	ids, paths := f.upload(t, 1)
	atCap := strings.Repeat("a", wire.MaxSessionPromptBytes)
	if res := f.promptImages(t, atCap+"b", ids...); !res.Refused || res.Reason != wire.ReasonPromptTextTooLong {
		t.Fatalf("over the cap = %+v, want %s", res, wire.ReasonPromptTextTooLong)
	}
	if res := f.promptImages(t, atCap, ids...); res.Refused {
		t.Fatalf("at the cap = %+v", res)
	}
	// Text this long goes in as one paste of its own.
	waitForSubmitted(t, f.conv, strconv.Quote(pasted(paths[0])+"\x1b[200~ "+atCap+"\x1b[201~")+"\n")
}

func TestSessionPromptImages_SlashCommandPastesImagesThenTypesItOnce(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	ids, paths := f.upload(t, 2)
	if res := f.promptImages(t, "/review", ids...); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, wantEntry("/review", paths...))
}

// Two swallowed Enters: the first submit's, then the nudge's bare Enter. The
// screen did not change in between, so the nudge clears and types the prompt
// again, and the images have to be pasted again with it.
func TestSessionPromptImages_RetypePastesTheImagesAgain(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	ids, paths := f.upload(t, 2)
	for range 2 {
		if err := agentfake.SendProgramControl(f.conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
			t.Fatal(err)
		}
	}
	if res := f.promptImages(t, "retyped", ids...); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, wantEntry("retyped", paths...))
}

func TestSessionPromptImages_DeleteRefusesASentImageOnly(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	ids, paths := f.upload(t, 2)
	if res := f.promptImages(t, "one", ids[0]); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, wantEntry("one", paths[0]))
	if res := f.deleteImage(t, ids[0]); !res.Refused || res.Reason != wire.ReasonImageSent || res.Message == "" {
		t.Errorf("delete of a sent image = %+v, want %s", res, wire.ReasonImageSent)
	}
	if res := f.deleteImage(t, ids[1]); res.Refused {
		t.Errorf("delete of an unsent image = %+v", res)
	}
}
