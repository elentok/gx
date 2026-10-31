package codexdoctor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRecord_WritesFixtureFormatThatReadsBack(t *testing.T) {
	dir := t.TempDir()
	want := Recording{
		Frames:   []Frame{{T: 0.04, Data: []byte("\x1b]0;gx\a")}, {T: 1.5, Data: []byte("\x1b]7501;state=idle:app=codex\a")}},
		Rollouts: []Rollout{{Name: "/tmp/sessions/2026/rollout-a.jsonl", Data: []byte("{\"type\":\"session_meta\"}\n")}},
	}
	src := Record(Source{Session: func(string) (Recording, error) { return want, nil }}, dir)
	if _, err := src.Session(TurnSession); err != nil {
		t.Fatal(err)
	}

	got, err := ReadStream(filepath.Join(dir, "streams", TurnSession+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].T != 1.5 || !bytes.Equal(got[0].Data, want.Frames[0].Data) {
		t.Errorf("ReadStream = %v, want %v", got, want.Frames)
	}
	data, err := os.ReadFile(filepath.Join(dir, "rollouts", "rollout-a.jsonl"))
	if err != nil || !bytes.Equal(data, want.Rollouts[0].Data) {
		t.Errorf("rollout = %q, %v, want the recorded bytes under its base name", data, err)
	}
}

func TestRecord_SkipsTheRolloutsSession(t *testing.T) {
	dir := t.TempDir()
	rec := Recording{Rollouts: []Rollout{{Name: "rollout-fixture.jsonl", Data: []byte("{}\n")}}}
	src := Record(Source{Session: func(string) (Recording, error) { return rec, nil }}, dir)
	if _, err := src.Session(RolloutsSession); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("dir holds %v, want nothing: the shipped fixtures are not re-recorded", entries)
	}
}

func TestReadStream_ReadsTheShippedFixtures(t *testing.T) {
	frames, err := ReadStream("testdata/streams/plain-turn.jsonl")
	if err != nil || len(frames) == 0 || len(frames[0].Data) == 0 {
		t.Fatalf("ReadStream = %d frames, %v", len(frames), err)
	}
}
