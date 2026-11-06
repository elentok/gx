package prompt_test

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/elentok/gx/server/servertest"
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

func TestSessionImage_NumberIgnoresSentMarkers(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	first := f.addImage(t, f.id, imageBytes(pngHead, 16))
	marker := filepath.Join(f.h.SessionsDir, f.id, "images", ".sent-"+first.ID)
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	second := f.addImage(t, f.id, imageBytes(pngHead, 16))
	if second.Name[:6] != "img-2-" {
		t.Errorf("second image = %q, want img-2-*", second.Name)
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

// serveImage is a signed-in GET of the serve route.
func (f promptFixture) serveImage(t *testing.T, session, image string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.base+"/v1/sessions/images?session="+url.QueryEscape(session)+"&id="+url.QueryEscape(image), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(f.ck)
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func TestSessionImage_ServeReturnsBytesWithHeaders(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	data := imageBytes(pngHead, 64)
	saved := f.addImage(t, f.id, data)
	resp, body := f.serveImage(t, f.id, saved.ID)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, data) {
		t.Fatalf("serve = %d, %d bytes", resp.StatusCode, len(body))
	}
	for k, want := range map[string]string{
		"Content-Type":           "image/png",
		"X-Content-Type-Options": "nosniff",
		"Cache-Control":          "private, max-age=31536000, immutable",
	} {
		if got := resp.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestSessionImage_ServeRefusesWhatIsNotInTheSessionFolder(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	saved := f.addImage(t, f.id, imageBytes(pngHead, 16))
	// A file next to the images folder must stay out of reach by any id.
	if err := os.WriteFile(filepath.Join(f.h.SessionsDir, f.id, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	other, err := f.h.Client.Sessions.Create(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || other.Session == nil {
		t.Fatalf("create = %+v, %v", other, err)
	}
	for name, tc := range map[string]struct{ session, image string }{
		"dot dot":         {f.id, "../secret.txt"},
		"dot dot to meta": {f.id, ".."},
		"absolute":        {f.id, filepath.Join(f.h.SessionsDir, f.id, "secret.txt")},
		"file name":       {f.id, saved.Name},
		"unknown id":      {f.id, "00000000"},
		"empty id":        {f.id, ""},
		"another session": {other.Session.ID, saved.ID},
		"unknown session": {"nope", saved.ID},
		"session dot dot": {"../" + f.id, saved.ID},
	} {
		if resp, _ := f.serveImage(t, tc.session, tc.image); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: serve = %d, want 404", name, resp.StatusCode)
		}
	}
}

func TestSessionImage_ServeNeedsSignIn(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	saved := f.addImage(t, f.id, imageBytes(pngHead, 16))
	if code, _ := servertest.WebDoCookie(t, http.MethodGet, f.base+"/v1/sessions/images?session="+f.id+"&id="+saved.ID, nil); code != http.StatusUnauthorized {
		t.Errorf("signed-out serve = %d, want %d", code, http.StatusUnauthorized)
	}
}

func TestSessionImage_DeleteRemovesAnUnsentFile(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	saved := f.addImage(t, f.id, imageBytes(pngHead, 16))
	path := filepath.Join(f.h.SessionsDir, f.id, "images", saved.Name)
	req := wire.SessionImageRequest{ID: f.id, Image: saved.ID}
	if res, err := f.h.Client.Sessions.RemoveImage(f.ctx, req); err != nil || res.Refused {
		t.Fatalf("delete = %+v, %v", res, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still there (err %v)", err)
	}
	if res, err := f.h.Client.Sessions.RemoveImage(f.ctx, req); err != nil || !res.Refused || res.Reason != wire.ReasonImageNotFound {
		t.Errorf("second delete = %+v, %v, want %s", res, err, wire.ReasonImageNotFound)
	}
	if resp, _ := f.serveImage(t, f.id, saved.ID); resp.StatusCode != http.StatusNotFound {
		t.Errorf("serve after delete = %d, want 404", resp.StatusCode)
	}
}

func TestSessionImage_DeleteSessionGates(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	saved := f.addImage(t, f.id, imageBytes(pngHead, 16))
	noChat, err := f.h.Client.Sessions.Create(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.NoResumeHarnessKind), Ticket: "proj:map"})
	if err != nil || noChat.Session == nil {
		t.Fatalf("create = %+v, %v", noChat, err)
	}
	if res, err := f.h.Client.Sessions.Stop(f.ctx, f.id); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	for name, tc := range map[string]struct{ id, want string }{
		"unknown":        {"nope", wire.ReasonSessionUnknown},
		"exited":         {f.id, wire.ReasonSessionNotLive},
		"no chat reader": {noChat.Session.ID, wire.ReasonPromptHarnessNotSupported},
	} {
		res, err := f.h.Client.Sessions.RemoveImage(f.ctx, wire.SessionImageRequest{ID: tc.id, Image: saved.ID})
		if err != nil || !res.Refused || res.Reason != tc.want {
			t.Errorf("%s: delete = %+v, %v, want %s", name, res, err, tc.want)
		}
	}
	if _, err := os.Stat(filepath.Join(f.h.SessionsDir, f.id, "images", saved.Name)); err != nil {
		t.Errorf("a refused delete removed the file: %v", err)
	}
}

func TestSessionImage_RemovingAnExitedSessionRemovesItsImages(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	f.addImage(t, f.id, imageBytes(pngHead, 16))
	if res, err := f.h.Client.Sessions.Stop(f.ctx, f.id); err != nil || res.Refused {
		t.Fatalf("stop = %+v, %v", res, err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionExited })
	if res, err := f.h.Client.Sessions.Remove(f.ctx, f.id); err != nil || res.Refused {
		t.Fatalf("remove = %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(f.h.SessionsDir, f.id)); !os.IsNotExist(err) {
		t.Errorf("session folder still there (err %v)", err)
	}
}
