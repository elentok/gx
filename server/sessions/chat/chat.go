package chat

import (
	"log/slog"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/commandlist"
	"github.com/elentok/gx/server/wire"
)

// DefaultPollInterval is how often a Poll checks its transcript when Config
// leaves ChatPollInterval zero.
const DefaultPollInterval = 2 * time.Second

// DefaultStallThreshold is how long a Working session's transcript may stay
// untouched before the session counts as stalled.
const DefaultStallThreshold = 15 * time.Minute

// RestoreCrashWindow is how soon after a Restore a crash counts as the
// Restore failing. The auto-resume window, resumeCrashWindow, is another rule.
const RestoreCrashWindow = time.Minute

// Meta is what the chat reads of a session's saved meta.
type Meta struct {
	ID, Harness, Project, Dir, Ticket, ConversationID string
	Model, Effort                                     string
	Status                                            wire.SessionStatus
	Exit                                              string
	RestoreCrashed                                    bool
}

// Source is what the chat needs from the sessions that own the metas.
type Source interface {
	// SessionDir resolves a sessions-dir session; a Queue agent id is unknown.
	SessionDir(id string) (string, Meta, wire.SessionResult, error)
	// ReadableSession is SessionDir, else a live Queue agent for the reads.
	ReadableSession(id string) (string, Meta, wire.SessionResult, error)
	ReadMeta(dir string) (Meta, error)
	// CheckProjectDir returns a refusal when project or dir is not valid for a
	// new session; nil means ok.
	CheckProjectDir(project, dir string) (*wire.SessionResult, error)
	// DesignFinal says whether the ticket's design is at its final step.
	DesignFinal(ticket string) bool
	// NoteFailure raises the banner of a session whose failure is new.
	NoteFailure(id string)
	// Stopped is closed when the session watches stop.
	Stopped() <-chan struct{}
	HasChatReader(harness string) bool
	CodexHome() (string, error)
	Publish(event, id string)
}

// Config holds the durations and paths the chat reads from the server config.
type Config struct {
	TranscriptMissTTL time.Duration
	ChatPollInterval  time.Duration
	StallThreshold    time.Duration
	CommandListAge    time.Duration
	Env               []string
	StateDir          string
}

// Deps is what New needs.
type Deps struct {
	Config Config
	Source Source
	Log    *slog.Logger
}

// Chat reads session transcripts, command lists, permission dialogs and
// screens.
type Chat struct {
	cfg Config
	src Source
	log *slog.Logger

	// chats caches parsed session chats by transcript file.
	chats Cache
	// transcripts caches where each conversation's transcript is.
	transcripts TranscriptPaths
	// commandLists caches the harnesses' command lists.
	commandLists *commandlist.Cache
	// authFailures caches whether each transcript ends in an auth error.
	authFailures AuthFailures
	// reportedModels caches the model ID on each transcript's newest reply.
	reportedModels FileCache[string]
	// codexTurns caches the model and effort on each rollout's newest turn.
	codexTurns FileCache[codexsession.TurnModel]
}

// New builds a Chat.
func New(d Deps) *Chat {
	return &Chat{
		cfg: d.Config, src: d.Source, log: d.Log,
		commandLists: commandlist.NewCache(d.Config.CommandListAge, nil),
	}
}

// ReadSnapshot reads the chat at path, as the page would see it. sessionDir is
// the session's state dir.
func (c *Chat) ReadSnapshot(path string, kind agentrunner.Kind, ticket, sessionDir string) (Snapshot, error) {
	final := c.src.DesignFinal(ticket)
	return c.chats.Read(path, kind, final, Loader(kind, ticket, sessionDir, final))
}

func refuse(reason, msg string) wire.SessionResult {
	return wire.SessionResult{Refused: true, Reason: reason, Message: msg}
}
