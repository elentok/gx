package prompt_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

const imageLimit = 20 << 20

// imageBytes is a file that starts with head and is size bytes long.
func imageBytes(head string, size int) []byte {
	return append([]byte(head), make([]byte, size-len(head))...)
}

var (
	pngHead  = "\x89PNG\r\n\x1a\n"
	jpegHead = "\xff\xd8\xff\xe0\x00\x10JFIF"
	gifHead  = "GIF89a"
	webpHead = "RIFF\x00\x00\x00\x00WEBPVP8 "
)

func (f promptFixture) addImage(t *testing.T, id string, data []byte) wire.SessionImageResult {
	t.Helper()
	res, err := f.h.Client.Sessions.AddImage(f.ctx, id, data)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSessionImage_SavesEachTypeUnchanged(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	name := regexp.MustCompile(`^img-\d+-[0-9a-f]+\.`)
	for ext, head := range map[string]string{".png": pngHead, ".jpg": jpegHead, ".gif": gifHead, ".webp": webpHead} {
		data := imageBytes(head, 64)
		res := f.addImage(t, f.id, data)
		if res.Refused || res.ID == "" || !name.MatchString(res.Name) || filepath.Ext(res.Name) != ext {
			t.Fatalf("%s: upload = %+v", ext, res)
		}
		got, err := os.ReadFile(filepath.Join(f.h.SessionsDir, f.id, "images", res.Name))
		if err != nil || !bytes.Equal(got, data) {
			t.Errorf("%s: saved file differs from the upload (err %v)", ext, err)
		}
	}
}

func TestSessionImage_NumbersImagesInOrder(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	first := f.addImage(t, f.id, imageBytes(pngHead, 16))
	second := f.addImage(t, f.id, imageBytes(pngHead, 16))
	if first.Name[:6] != "img-1-" || second.Name[:6] != "img-2-" || first.ID == second.ID {
		t.Errorf("names = %q, %q", first.Name, second.Name)
	}
}

func TestSessionImage_RefusesOtherTypes(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	for name, data := range map[string][]byte{
		"text":  []byte("this is not a png"),
		"heic":  []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"),
		"svg":   []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"empty": nil,
	} {
		res := f.addImage(t, f.id, data)
		if !res.Refused || res.Reason != wire.ReasonImageTypeNotAllowed {
			t.Errorf("%s: upload = %+v, want %s", name, res, wire.ReasonImageTypeNotAllowed)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(f.h.SessionsDir, f.id, "images")); len(entries) != 0 {
		t.Errorf("a refused upload left %d files", len(entries))
	}
}

func TestSessionImage_SizeLimit(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	max := imageBytes(pngHead, imageLimit)
	res := f.addImage(t, f.id, max)
	if res.Refused {
		t.Fatalf("exactly 20 MB = %+v, want saved", res)
	}
	if got, err := os.ReadFile(filepath.Join(f.h.SessionsDir, f.id, "images", res.Name)); err != nil || !bytes.Equal(got, max) {
		t.Errorf("saved 20 MB file differs (err %v)", err)
	}
	if res := f.addImage(t, f.id, imageBytes(pngHead, imageLimit+1)); !res.Refused || res.Reason != wire.ReasonImageTooLarge {
		t.Errorf("20 MB + 1 byte = %+v, want %s", res, wire.ReasonImageTooLarge)
	}
	if res := f.addImage(t, f.id, imageBytes(pngHead, imageLimit+1<<20)); !res.Refused || res.Reason != wire.ReasonImageTooLarge {
		t.Errorf("past the body cap = %+v, want %s", res, wire.ReasonImageTooLarge)
	}
}

func TestSessionImage_SessionGates(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	// A seeded session names a harness with no chat reader, so the Exited
	// case stops a real one.
	if res, err := f.h.Client.Sessions.Stop(f.ctx, f.id); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	noChat, err := f.h.Client.Sessions.Create(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.NoResumeHarnessKind), Ticket: "proj:map"})
	if err != nil || noChat.Session == nil {
		t.Fatalf("create = %+v, %v", noChat, err)
	}
	for name, tc := range map[string]struct{ id, want string }{
		"unknown":        {"nope", wire.ReasonSessionUnknown},
		"exited":         {f.id, wire.ReasonSessionNotLive},
		"no chat reader": {noChat.Session.ID, wire.ReasonPromptHarnessNotSupported},
	} {
		if res := f.addImage(t, tc.id, imageBytes(pngHead, 16)); !res.Refused || res.Reason != tc.want {
			t.Errorf("%s: upload = %+v, want %s", name, res, tc.want)
		}
	}
}

func TestSessionImage_WorksWhileWorking(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	report(t, f.conv, agentfake.Control{State: "working"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	if res := f.addImage(t, f.id, imageBytes(pngHead, 16)); res.Refused {
		t.Errorf("upload while working = %+v", res)
	}
}
