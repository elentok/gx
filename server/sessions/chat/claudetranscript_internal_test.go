package chat

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// fakeSource is a Source that serves one session and records what the chat
// asks it to publish and note.
type fakeSource struct {
	dir       string
	meta      Meta
	res       wire.SessionResult
	refusal   *wire.SessionResult
	final     bool
	readers   map[string]bool
	codexHome string
	stopped   chan struct{}
	noted     chan string
	published chan [2]string
}

func newFakeSource() *fakeSource {
	return &fakeSource{
		readers: map[string]bool{"claude": true, "codex": true}, stopped: make(chan struct{}),
		noted: make(chan string, 16), published: make(chan [2]string, 16),
	}
}

func (f *fakeSource) SessionDir(string) (string, Meta, wire.SessionResult, error) {
	return f.dir, f.meta, f.res, nil
}
func (f *fakeSource) ReadableSession(string) (string, Meta, wire.SessionResult, error) {
	return f.dir, f.meta, f.res, nil
}
func (f *fakeSource) ReadMeta(string) (Meta, error) { return f.meta, nil }
func (f *fakeSource) CheckProjectDir(string, string) (*wire.SessionResult, error) {
	return f.refusal, nil
}
func (f *fakeSource) DesignFinal(string) bool     { return f.final }
func (f *fakeSource) NoteFailure(id string)       { f.noted <- id }
func (f *fakeSource) Stopped() <-chan struct{}    { return f.stopped }
func (f *fakeSource) HasChatReader(h string) bool { return f.readers[h] }
func (f *fakeSource) CodexHome() (string, error)  { return f.codexHome, nil }
func (f *fakeSource) Publish(event, id string)    { f.published <- [2]string{event, id} }

func claudeChat(src Source, config string) *Chat {
	return New(Deps{
		Config: Config{Env: []string{"CLAUDE_CONFIG_DIR=" + config}},
		Source: src, Log: slog.New(slog.DiscardHandler),
	})
}

// A copy of the transcript under another project dir makes a glob ambiguous
// and so a miss: finding the expected one proves the glob was never run.
func TestClaudeTranscript_StatsTheExpectedSlugDirBeforeGlobbing(t *testing.T) {
	t.Parallel()
	config := t.TempDir()
	cwd := "/work/proj"
	want := filepath.Join(config, "projects", transcript.Slugify(cwd), "conv.jsonl")
	other := filepath.Join(config, "projects", "-elsewhere", "conv.jsonl")
	for _, p := range []string{want, other} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := claudeChat(newFakeSource(), config)
	got, ok := c.ClaudeTranscript("s1", Meta{Dir: cwd, ConversationID: "conv"})
	if !ok || got != want {
		t.Fatalf("ClaudeTranscript = %q, %v, want %q", got, ok, want)
	}
}

func TestClaudeTranscript_ReadsTheConfigDirFromTheServerEnv(t *testing.T) {
	t.Parallel()
	config := t.TempDir()
	cwd := "/work/proj"
	want := filepath.Join(config, "projects", transcript.Slugify(cwd), "conv.jsonl")
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := claudeChat(newFakeSource(), config)
	got, ok := c.ClaudeTranscript("s1", Meta{Dir: cwd, ConversationID: "conv"})
	if !ok || got != want {
		t.Fatalf("ClaudeTranscript = %q, %v, want %q", got, ok, want)
	}
}

func TestFailure_StatusDecidesBeforeTheTranscript(t *testing.T) {
	t.Parallel()
	src := newFakeSource()
	c := claudeChat(src, t.TempDir())
	m := Meta{Harness: "claude", Status: wire.SessionStatus{State: wire.SessionRateLimited, Message: "later"}}
	f := c.Failure(m)
	if f == nil || f.Kind != wire.FailureKindRateLimit || f.Text != "later" {
		t.Fatalf("Failure = %+v, want a rate-limit failure with the status message", f)
	}
	if c.Failure(Meta{Harness: "claude", Status: wire.SessionStatus{State: wire.SessionIdle}}) != nil {
		t.Fatal("an idle session without a transcript has no failure")
	}
}

func TestPoll_PublishesOnTranscriptGrowth(t *testing.T) {
	t.Parallel()
	config := t.TempDir()
	cwd := "/work/proj"
	path := filepath.Join(config, "projects", transcript.Slugify(cwd), "conv.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := newFakeSource()
	src.meta = Meta{ID: "s1", Harness: "claude", Dir: cwd, ConversationID: "conv"}
	c := claudeChat(src, config)
	c.cfg.ChatPollInterval = 5 * 1e6
	p := c.NewPoll("dir", "s1")
	defer p.Close()
	p.SetActive(true)
	<-src.noted
	<-src.noted // the first tick has finished, so the baseline size is taken
	if err := os.WriteFile(path, []byte("{}\n{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := <-src.published
	if got != [2]string{wire.EventSessionChatChanged, "s1"} {
		t.Fatalf("published %v, want a chat-changed event for s1", got)
	}
}
