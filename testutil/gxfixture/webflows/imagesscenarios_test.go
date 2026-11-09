package webflows

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elentok/gx/server/imagestore"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func TestDesignImages_ListsTheMapsAttachedImages(t *testing.T) {
	t.Parallel()
	r := start(t, "design-images")
	defer r.Stop(t)
	listed := imagestore.List(filepath.Join(r.Root(), "data", "gx", "tickets", "demo", "design-epic", "images"))
	if len(listed) != spec.DesignImagesCount {
		t.Errorf("attached images = %d, want %d", len(listed), spec.DesignImagesCount)
	}
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Error("no design session is open")
	}
}

func TestAgentsImages_ClaudeAndCodexChatsListSavedImages(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-images")
	defer r.Stop(t)
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.Gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	// Per message: saved images, then a placeholder flag.
	want := []string{"3", "2", "0", "placeholder", "10"}
	harnesses := map[string]int{}
	for _, si := range list {
		var chat wire.SessionChatResponse
		if err := json.Unmarshal([]byte(r.Gx(t, "agents", "chat", si.ID, "--json")), &chat); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, m := range chat.Messages {
			switch {
			case len(m.Images) == 1 && m.Images[0].Placeholder:
				got = append(got, "placeholder")
			case m.Role == "person":
				got = append(got, map[int]string{0: "0", 2: "2", 3: "3", 10: "10"}[len(m.Images)])
			}
		}
		if len(got) < len(want) || !reflect.DeepEqual(got[len(got)-len(want):], want) {
			continue
		}
		harnesses[si.Harness]++
		entries, err := os.ReadDir(filepath.Join(r.Root(), "data", "gx", "sessions", si.ID, "images"))
		if err != nil || len(entries) != 15 {
			t.Errorf("session %s: saved images %d, err %v; want 15", si.ID, len(entries), err)
		}
	}
	if harnesses["claude"] != 1 || harnesses["codex"] != 1 {
		t.Errorf("sessions with the five image messages by harness = %v, want one claude and one codex", harnesses)
	}
}
