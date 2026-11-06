package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestAgentsImages_ClaudeAndCodexChatsListSavedImages(t *testing.T) {
	t.Parallel()
	r := start(t, "agents-images")
	defer r.stop(t)
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(r.gx(t, "agents", "list", "--json")), &list); err != nil {
		t.Fatal(err)
	}
	// Per message: saved images, then a placeholder flag.
	want := []string{"3", "2", "0", "placeholder", "10"}
	harnesses := map[string]int{}
	for _, si := range list {
		var chat wire.SessionChatResponse
		if err := json.Unmarshal([]byte(r.gx(t, "agents", "chat", si.ID, "--json")), &chat); err != nil {
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
		entries, err := os.ReadDir(filepath.Join(fixtureRoot(r), "data", "gx", "sessions", si.ID, "images"))
		if err != nil || len(entries) != 15 {
			t.Errorf("session %s: saved images %d, err %v; want 15", si.ID, len(entries), err)
		}
	}
	if harnesses["claude"] != 1 || harnesses["codex"] != 1 {
		t.Errorf("sessions with the five image messages by harness = %v, want one claude and one codex", harnesses)
	}
}
