package designs_test

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

// webGetRaw is a web GET that returns the status, Content-Type and body.
func (f designsFixture) webGetRaw(t *testing.T, path string) (int, string, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(f.ck)
	resp, err := servertest.WebClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), b
}

func TestWebDesignDraftImages_Serve(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "s", "idea")
	cases := []struct {
		name, contentType string
		data              []byte
	}{
		{"png", "image/png", imageBytes("\x89PNG\r\n\x1a\n", 40)},
		{"jpeg", "image/jpeg", imageBytes("\xff\xd8\xff\xe0", 40)},
		{"gif", "image/gif", imageBytes("GIF89a", 40)},
		{"webp", "image/webp", append(append([]byte("RIFF\x00\x00\x00\x00WEBP"), 'V', 'P', '8', ' '), make([]byte, 20)...)},
	}
	for _, c := range cases {
		up := f.uploadDraftImage(t, d.ID, c.data)
		code, ct, body := f.webGetRaw(t, "/v1/designs/drafts/images?id="+url.QueryEscape(d.ID)+"&image="+url.QueryEscape(up.ID))
		if code != http.StatusOK || ct != c.contentType || string(body) != string(c.data) {
			t.Errorf("%s: GET = %d %q (%d bytes), want 200 %q", c.name, code, ct, len(body), c.contentType)
		}
	}

	for name, path := range map[string]string{
		"unknown image": "/v1/designs/drafts/images?id=" + d.ID + "&image=00000000",
		"traversal":     "/v1/designs/drafts/images?id=" + d.ID + "&image=..%2F..%2Fdraft.json",
		"unknown draft": "/v1/designs/drafts/images?id=0123456789abcdef&image=00000000",
		"bad draft id":  "/v1/designs/drafts/images?id=..&image=00000000",
	} {
		if code, _, _ := f.webGetRaw(t, path); code != http.StatusNotFound {
			t.Errorf("%s: GET = %d, want 404", name, code)
		}
	}
}

func TestWebDesignDraftImages_ServeGoneDraft(t *testing.T) {
	t.Parallel()
	f := startDesigns(t)
	d := f.seedDraft(t, "proj", "s", "idea")
	up := f.uploadDraftImage(t, d.ID, imageBytes("\x89PNG\r\n\x1a\n", 40))
	path := "/v1/designs/drafts/images?id=" + d.ID + "&image=" + up.ID
	f.postDraftDelete(t, d.ID)
	if code, _, _ := f.webGetRaw(t, path); code != http.StatusNotFound {
		t.Errorf("GET after delete = %d, want 404", code)
	}
}

func (f designsFixture) postDraftDelete(t *testing.T, id string) {
	t.Helper()
	code, out := servertest.WebPostJSON(t, f.base+"/v1/designs/drafts/delete", `{"id":"`+id+`"}`, f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK {
		t.Fatalf("delete = %d %s", code, out)
	}
}

func TestWebDesignMapImages(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	servertest.WriteTicketWith(t, store, "proj", "map", "01", "first", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, store, "proj", "plain", "01", "first", servertest.TicketOpts{})
	servertest.WriteTicketWith(t, store, "proj", "bare", "01", "first", servertest.TicketOpts{})
	for _, epic := range []string{"map", "bare"} {
		if err := os.WriteFile(filepath.Join(store, "proj", epic, "ticket.md"), []byte("---\nkind: map\n---\nsecret\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	png := imageBytes("\x89PNG\r\n\x1a\n", 40)
	imgs := filepath.Join(store, "proj", "map", "images")
	if err := os.MkdirAll(imgs, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"img-2-bbbbbbbb.png": png,
		"img-1-aaaaaaaa.png": png,
		"notes.txt":          []byte("not an image gx wrote"),
	} {
		if err := os.WriteFile(filepath.Join(imgs, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := startDesignsIn(t, store)

	var list []wire.SessionImageResult
	f.webGet(t, "/v1/designs/images?epic=proj:map", &list)
	if len(list) != 2 || list[0].ID != "aaaaaaaa" || list[1].ID != "bbbbbbbb" || list[0].Name != "img-1-aaaaaaaa.png" {
		t.Errorf("list = %+v, want the two images in order", list)
	}

	code, ct, body := f.webGetRaw(t, "/v1/designs/images/read?epic=proj:map&id=aaaaaaaa")
	if code != http.StatusOK || ct != "image/png" || string(body) != string(png) {
		t.Errorf("serve = %d %q, want 200 image/png", code, ct)
	}

	for name, path := range map[string]string{
		"unknown id":        "/v1/designs/images/read?epic=proj:map&id=cccccccc",
		"traversal id":      "/v1/designs/images/read?epic=proj:map&id=..%2Fticket.md",
		"ticket.md":         "/v1/designs/images/read?epic=proj:map&id=ticket.md",
		"non-image file":    "/v1/designs/images/read?epic=proj:map&id=notes.txt",
		"traversal epic":    "/v1/designs/images/read?epic=proj:..%2Fmap&id=aaaaaaaa",
		"no images folder":  "/v1/designs/images/read?epic=proj:bare&id=aaaaaaaa",
		"non-map epic":      "/v1/designs/images/read?epic=proj:plain&id=aaaaaaaa",
		"unknown project":   "/v1/designs/images/read?epic=nope:map&id=aaaaaaaa",
		"unknown epic":      "/v1/designs/images/read?epic=proj:nope&id=aaaaaaaa",
		"malformed address": "/v1/designs/images/read?epic=map&id=aaaaaaaa",
	} {
		if code, _, _ := f.webGetRaw(t, path); code != http.StatusNotFound {
			t.Errorf("%s: GET = %d, want 404", name, code)
		}
	}

	for _, epic := range []string{"proj:bare", "proj:plain", "nope:map", "proj:..%2Fmap"} {
		var got []wire.SessionImageResult
		f.webGet(t, "/v1/designs/images?epic="+epic, &got)
		if len(got) != 0 {
			t.Errorf("list %s = %+v, want empty", epic, got)
		}
	}
}
