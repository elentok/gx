package limits

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tokenledger"
	"github.com/elentok/gx/transcript"
)

// DefaultPoll is the poll interval of the budget and the token ledger when
// their config leaves it unset.
const DefaultPoll = 30 * time.Second

// TokenHost is what the token poll reads from the root.
type TokenHost interface {
	// Snapshot is the ticket index's snapshot.
	Snapshot() wire.Snapshot
	// Sessions lists the interactive sessions.
	Sessions() ([]InteractiveSession, error)
	// OnMapEpic reports whether address is on a map epic.
	OnMapEpic(address string) bool
	// BudgetIteration is the live iteration of address as its runner sees it.
	BudgetIteration(address string) (wire.IterationInfo, bool)
	// MigrationsDone closes once the migrations end, passed or failed.
	MigrationsDone() <-chan struct{}
}

// InteractiveSession is the part of a session's meta the token poll reads.
type InteractiveSession struct {
	ID             string
	Harness        string
	Project        string
	Dir            string
	Ticket         string
	ConversationID string
	Stopped        bool
	Exit           string
}

// TokenReader reads a session's cumulative tokens and dollars.
type TokenReader func(ref ralphloop.SessionRef) (tokens int64, cost float64, err error)

// transcriptSig is what an idle session keeps unchanged: its transcript's size
// and mtime, and those of its subagent transcripts.
type transcriptSig struct {
	size  int64
	mtime time.Time
	subs  string
}

// tokenPoller holds the poll's caches. The zero value works.
type tokenPoller struct {
	mu      sync.Mutex
	read    TokenReader
	sigs    map[string]transcriptSig // session key -> last polled signature
	rollout map[string]string        // Codex session id -> rollout path
	groups  map[string]string        // interactive session key -> group, fixed at first sight
}

// tokenSession is one iteration session to meter.
type tokenSession struct {
	key     string // ledger key: address@session
	project string
	group   string
	harness string
	ref     ralphloop.SessionRef
	path    string // transcript or rollout whose change means new usage
}

func (l *Limits) readTokens(ref ralphloop.SessionRef) (int64, float64, error) {
	if f := l.tokenPoll.read; f != nil {
		return f(ref)
	}
	var homes ralphloop.SessionHomes
	homes.Claude, _ = l.projects.EnvHome()
	homes.Codex, _ = l.projects.CodexHome()
	return ralphloop.SessionsTotals(homes, []ralphloop.SessionRef{ref})
}

// tokenSessionOf resolves address's live iteration session, false when it has
// none yet.
func (l *Limits) tokenSessionOf(address string) (tokenSession, bool) {
	a, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return tokenSession{}, false
	}
	group := tokenledger.GroupQueue
	if a.Epic == tickets.OneOffsEpic {
		group = tokenledger.GroupOneOff
	}
	label, _, worktree := ralphloop.IterationIdentity(a.Epic, a.ID, l.projects.WorktreeDir(a.Project))
	agentDir := filepath.Join(nativerunner.AgentsRoot(l.cfg.StateDir, a.Project), label)
	run, _ := l.registry.RunOf(address)
	codex := IsCodexRun(run)
	if d, err := nativerunner.OpenDir(agentDir); err == nil && agentrunner.Kind(d.Meta.Harness) == harness.Codex {
		codex = true
	}
	ts := tokenSession{project: a.Project, group: group}
	if codex {
		id := harness.ReadCodexSessionID(agentDir)
		home, err := l.projects.CodexHome()
		if id == "" || err != nil {
			return tokenSession{}, false
		}
		path, ok := l.rolloutPath(home, id)
		if !ok {
			return tokenSession{}, false
		}
		ts.harness, ts.path = string(harness.Codex), path
		ts.ref = ralphloop.SessionRef{Agent: ralphloop.AgentCodex, Cwd: worktree, SessionID: id}
	} else {
		it, ok := l.tokenHost.BudgetIteration(address)
		home, err := l.projects.EnvHome()
		if !ok || it.Session == "" || err != nil {
			return tokenSession{}, false
		}
		ts.harness, ts.path = string(harness.Claude), transcript.PathIn(home, it.Worktree, it.Session)
		ts.ref = ralphloop.SessionRef{Agent: ralphloop.AgentClaude, Cwd: it.Worktree, SessionID: it.Session}
	}
	ts.key = address + "@" + ts.ref.SessionID
	return ts, true
}

// rolloutPath finds a Codex rollout once; the lookup walks the sessions tree.
func (l *Limits) rolloutPath(home, id string) (string, bool) {
	p := &l.tokenPoll
	p.mu.Lock()
	path, ok := p.rollout[id]
	p.mu.Unlock()
	if ok {
		return path, true
	}
	path, ok, err := codexsession.RolloutPathIn(home, id)
	if err != nil || !ok {
		return "", false
	}
	p.mu.Lock()
	if p.rollout == nil {
		p.rollout = map[string]string{}
	}
	p.rollout[id] = path
	p.mu.Unlock()
	return path, true
}

// changed reports whether the session's files differ from the last poll, and
// remembers the new signature.
func (p *tokenPoller) changed(ts tokenSession) bool {
	sig, ok := signatureOf(ts.path)
	if !ok {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if old, seen := p.sigs[ts.key]; seen && old == sig {
		return false
	}
	if p.sigs == nil {
		p.sigs = map[string]transcriptSig{}
	}
	p.sigs[ts.key] = sig
	return true
}

// PollTokensAt adds every live iteration's token and dollar change since its
// last poll to the day of now in the token ledger. A session seen for the first
// time only stores its total.
func (l *Limits) PollTokensAt(now time.Time) {
	day := now.Format(tokenledger.DayLayout)
	for _, t := range l.tokenHost.Snapshot().Tickets {
		if t.Status != "claimed" && !l.registry.Has(t.Address) {
			continue
		}
		if ts, ok := l.tokenSessionOf(t.Address); ok {
			l.pollSession(ts, day)
		}
	}
	l.pollInteractiveTokens(day)
}

// pollSession adds one session's change since its last poll to the ledger.
func (l *Limits) pollSession(ts tokenSession, day string) {
	if !l.tokenPoll.changed(ts) {
		return
	}
	tokens, cost, err := l.readTokens(ts.ref)
	if err != nil {
		return
	}
	cur := tokenledger.Seen{Tokens: tokens, Dollars: cost}
	prev, seen, err := l.tokens.Seen(ts.key)
	if err != nil {
		l.log.Error("read token ledger", "err", err)
		return
	}
	// A first-seen session adds nothing; it only sets its seen total.
	d := tokenledger.Delta{Key: tokenledger.Key{Day: day, Project: ts.project, Group: ts.group, Harness: ts.harness}}
	if seen {
		d.Tokens, d.Dollars = max(tokens-prev.Tokens, 0), max(cost-prev.Dollars, 0)
	}
	if err := l.tokens.Record(d, ts.key, cur); err != nil {
		l.log.Error("save token ledger", "err", err)
	}
}

// pollInteractiveTokens meters every live interactive session. Its group is
// fixed at first sight: Design when its Session ticket is on a map, else Other.
func (l *Limits) pollInteractiveTokens(day string) {
	infos, err := l.tokenHost.Sessions()
	if err != nil {
		l.log.Error("list sessions for token poll", "err", err)
		return
	}
	for _, m := range infos {
		if m.Stopped || m.Exit != "" || m.ConversationID == "" {
			continue
		}
		ts, ok := l.interactiveTokenSession(m)
		if ok {
			l.pollSession(ts, day)
		}
	}
}

func (l *Limits) interactiveTokenSession(m InteractiveSession) (tokenSession, bool) {
	ts := tokenSession{
		key:     "session/" + m.ID + "@" + m.ConversationID,
		project: m.Project,
		harness: m.Harness,
		ref:     ralphloop.SessionRef{Agent: ralphloop.AgentFor(m.Harness), Cwd: m.Dir, SessionID: m.ConversationID},
	}
	if agentrunner.Kind(m.Harness) == harness.Codex {
		home, err := l.projects.CodexHome()
		if err != nil {
			return tokenSession{}, false
		}
		path, ok := l.rolloutPath(home, m.ConversationID)
		if !ok {
			return tokenSession{}, false
		}
		ts.path = path
	} else {
		home, err := l.projects.EnvHome()
		if err != nil {
			return tokenSession{}, false
		}
		ts.path = transcript.PathIn(home, m.Dir, m.ConversationID)
	}
	ts.group = l.tokenPoll.groupOf(ts.key, func() string {
		if m.Ticket != "" && l.tokenHost.OnMapEpic(m.Ticket) {
			return tokenledger.GroupDesign
		}
		return tokenledger.GroupOther
	})
	return ts, true
}

// groupOf returns key's group, computing it once.
func (p *tokenPoller) groupOf(key string, compute func() string) string {
	p.mu.Lock()
	if g, ok := p.groups[key]; ok {
		p.mu.Unlock()
		return g
	}
	p.mu.Unlock()
	g := compute()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.groups == nil {
		p.groups = map[string]string{}
	}
	p.groups[key] = g
	return g
}

// KeepTokensPolled polls once the migrations end, passed or failed: the ledger
// backfill is a migration step, and a poll before it would stamp a first live
// day that makes the step skip days.
func (l *Limits) KeepTokensPolled(ctx context.Context) {
	select {
	case <-l.tokenHost.MigrationsDone():
	case <-ctx.Done():
		return
	}
	interval := l.cfg.TokenPollInterval
	if interval <= 0 {
		interval = DefaultPoll
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			l.PollTokensAt(time.Now())
		}
	}
}

// SetTokensOf swaps how a session's cumulative tokens and cost are read.
func (l *Limits) SetTokensOf(f TokenReader) { l.tokenPoll.read = f }

// AddLedgerTokens writes a live delta straight into the token ledger.
func (l *Limits) AddLedgerTokens(d tokenledger.Delta) error { return l.tokens.Add(d) }

// TokenTranscriptPath is the file whose change makes the token poll read
// address's session; false when the ticket has no live session.
func (l *Limits) TokenTranscriptPath(address string) (string, bool) {
	ts, ok := l.tokenSessionOf(address)
	return ts.path, ok
}

// signatureOf stats the transcript and its subagent transcripts. ok is false
// when the transcript does not exist yet.
func signatureOf(path string) (transcriptSig, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return transcriptSig{}, false
	}
	sig := transcriptSig{size: fi.Size(), mtime: fi.ModTime()}
	subs, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "*.jsonl"))
	var b strings.Builder
	for _, f := range subs {
		if fi, err := os.Stat(f); err == nil {
			b.WriteString(f + "|" + fi.ModTime().String() + "|" + strconv.FormatInt(fi.Size(), 10) + ";")
		}
	}
	sig.subs = b.String()
	return sig, true
}
