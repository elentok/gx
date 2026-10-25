package server

import (
	"os"
	"time"

	"github.com/elentok/gx/transcript"
)

// defaultTurnSettleTimeout caps the wait for claude's end-of-turn line. A
// turn cut short by Esc may never write one.
const defaultTurnSettleTimeout = 2 * time.Second

const turnSettlePoll = 50 * time.Millisecond

// turnMark is a session's transcript as it stood at an idle edge.
type turnMark struct {
	path string
	size int64
}

// markTurn records the transcript of a session that just went idle. A session
// whose harness has no chat, or no transcript yet, is not marked.
func (s *Server) markTurn(dir, id string) (turnMark, bool) {
	meta, err := readSessionMeta(dir)
	if err != nil || meta.Harness != s.settings.Load().chatHarness() {
		return turnMark{}, false
	}
	path, ok := s.claudeTranscript(id, meta)
	if !ok {
		return turnMark{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return turnMark{}, false
	}
	return turnMark{path: path, size: info.Size()}, true
}

// settleTurn waits for claude to finish writing the turn that just ended.
// Claude reports idle before it flushes the turn's last lines, so the idle
// edge's own reads (the page's chat, the Design check) can miss the final
// message. When the transcript grew since mark, settleTurn redoes the Design
// check and announces the chat again. The session turning busy (cancel) ends
// the wait.
func (s *Server) settleTurn(dir, id string, mark turnMark, design designRef, cancel <-chan struct{}) {
	deadline := time.NewTimer(s.cfg.TurnSettleTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(turnSettlePoll)
	defer tick.Stop()
wait:
	for {
		if ended, err := transcript.TurnEnded(mark.path); err != nil || ended {
			break
		}
		select {
		case <-cancel:
			return
		case <-s.sessionWatches.stop:
			return
		case <-deadline.C:
			break wait
		case <-tick.C:
		}
	}
	info, err := os.Stat(mark.path)
	if err != nil || info.Size() == mark.size {
		return
	}
	if design.Conversation != "" {
		s.detectDesignActionable(dir, id, design)
	}
	s.events.publish(EventSessionChatChanged, id)
}
