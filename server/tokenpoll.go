package server

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
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tokenledger"
	"github.com/elentok/gx/transcript"
)

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

func (s *Server) readTokens(ref ralphloop.SessionRef) (int64, float64, error) {
	if f := s.tokenPoll.read; f != nil {
		return f(ref)
	}
	var homes ralphloop.SessionHomes
	homes.Claude, _ = s.envHome()
	homes.Codex, _ = s.codexHome()
	return ralphloop.SessionsTotals(homes, []ralphloop.SessionRef{ref})
}

// tokenSessionOf resolves address's live iteration session, false when it has
// none yet.
func (s *Server) tokenSessionOf(address string) (tokenSession, bool) {
	a, err := tickets.ParseAddress(address, tickets.AddressContext{})
	if err != nil {
		return tokenSession{}, false
	}
	group := tokenledger.GroupQueue
	if a.Epic == tickets.OneOffsEpic {
		group = tokenledger.GroupOneOff
	}
	label, _, worktree := ralphloop.IterationIdentity(a.Epic, a.ID, s.worktreeDir(a.Project))
	agentDir := filepath.Join(nativerunner.AgentsRoot(s.cfg.StateDir, a.Project), label)
	run, _ := s.registry.RunOf(address)
	codex := isCodexRun(run)
	if d, err := nativerunner.OpenDir(agentDir); err == nil && agentrunner.Kind(d.Meta.Harness) == harness.Codex {
		codex = true
	}
	ts := tokenSession{project: a.Project, group: group}
	if codex {
		id := harness.ReadCodexSessionID(agentDir)
		home, err := s.codexHome()
		if id == "" || err != nil {
			return tokenSession{}, false
		}
		path, ok := s.rolloutPath(home, id)
		if !ok {
			return tokenSession{}, false
		}
		ts.harness, ts.path = string(harness.Codex), path
		ts.ref = ralphloop.SessionRef{Agent: ralphloop.AgentCodex, Cwd: worktree, SessionID: id}
	} else {
		it, ok := s.budgetIteration(address)
		home, err := s.envHome()
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
func (s *Server) rolloutPath(home, id string) (string, bool) {
	p := &s.tokenPoll
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

// pollTokens adds every live iteration's token and dollar change since its last
// poll to today's bucket of the token ledger. A session seen for the first time
// only stores its total.
func (s *Server) pollTokens(now time.Time) {
	day := now.Format(tokenledger.DayLayout)
	for _, t := range s.idx.Snapshot().Tickets {
		if t.Status != "claimed" && !s.registry.Has(t.Address) {
			continue
		}
		if ts, ok := s.tokenSessionOf(t.Address); ok {
			s.pollSession(ts, day)
		}
	}
	s.pollInteractiveTokens(day)
}

// pollSession adds one session's change since its last poll to the ledger.
func (s *Server) pollSession(ts tokenSession, day string) {
	if !s.tokenPoll.changed(ts) {
		return
	}
	tokens, cost, err := s.readTokens(ts.ref)
	if err != nil {
		return
	}
	cur := tokenledger.Seen{Tokens: tokens, Dollars: cost}
	prev, seen, err := s.tokens.Seen(ts.key)
	if err != nil {
		s.log.Error("read token ledger", "err", err)
		return
	}
	// A first-seen session adds nothing; it only sets its seen total.
	d := tokenledger.Delta{Key: tokenledger.Key{Day: day, Project: ts.project, Group: ts.group, Harness: ts.harness}}
	if seen {
		d.Tokens, d.Dollars = max(tokens-prev.Tokens, 0), max(cost-prev.Dollars, 0)
	}
	if err := s.tokens.Record(d, ts.key, cur); err != nil {
		s.log.Error("save token ledger", "err", err)
	}
}

// pollInteractiveTokens meters every live interactive session. Its group is
// fixed at first sight: Design when its Session ticket is on a map, else Other.
func (s *Server) pollInteractiveTokens(day string) {
	infos, err := s.listSessionMetas()
	if err != nil {
		s.log.Error("list sessions for token poll", "err", err)
		return
	}
	for _, m := range infos {
		if m.Stopped || m.Exit != "" || m.ConversationID == "" {
			continue
		}
		ts, ok := s.interactiveTokenSession(m)
		if ok {
			s.pollSession(ts, day)
		}
	}
}

func (s *Server) interactiveTokenSession(m sessionMeta) (tokenSession, bool) {
	ts := tokenSession{
		key:     "session/" + m.ID + "@" + m.ConversationID,
		project: m.Project,
		harness: m.Harness,
		ref:     ralphloop.SessionRef{Agent: ralphloop.AgentFor(m.Harness), Cwd: m.Dir, SessionID: m.ConversationID},
	}
	if agentrunner.Kind(m.Harness) == harness.Codex {
		home, err := s.codexHome()
		if err != nil {
			return tokenSession{}, false
		}
		path, ok := s.rolloutPath(home, m.ConversationID)
		if !ok {
			return tokenSession{}, false
		}
		ts.path = path
	} else {
		home, err := s.envHome()
		if err != nil {
			return tokenSession{}, false
		}
		ts.path = transcript.PathIn(home, m.Dir, m.ConversationID)
	}
	ts.group = s.tokenPoll.groupOf(ts.key, func() string {
		if m.Ticket != "" && s.onMapEpic(m.Ticket) {
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

// keepTokensPolled polls once the migrations end, passed or failed: the ledger
// backfill is a migration step, and a poll before it would stamp a first live
// day that makes the step skip days.
func (s *Server) keepTokensPolled(ctx context.Context) {
	select {
	case <-s.migrationsDone:
	case <-ctx.Done():
		return
	}
	interval := s.cfg.TokenPollInterval
	if interval <= 0 {
		interval = defaultBudgetPoll
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.pollTokens(time.Now())
		}
	}
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
