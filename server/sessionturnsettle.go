package server

import (
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/wire"
	"os"
	"time"
)

// defaultTurnSettleTimeout caps the wait for claude's end-of-turn line. A
// turn cut short by Esc may never write one.
const defaultTurnSettleTimeout = 2 * time.Second

const turnSettlePoll = 50 * time.Millisecond

// markTurn records the transcript of a session that just went idle. A session
// whose harness has no chat, or no transcript yet, is not marked.
func (s *Server) markTurn(dir, id string) (chat.TurnMark, bool) {
	meta, err := sessions.ReadMeta(dir)
	if err != nil || !s.settings.Load().HasChatReader(meta.Harness) {
		return chat.TurnMark{}, false
	}
	path, kind, ok := s.sessionChat.Transcript(id, chatMeta(meta))
	if !ok {
		return chat.TurnMark{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return chat.TurnMark{}, false
	}
	return chat.TurnMark{Path: path, Size: info.Size(), Kind: kind}, true
}

// settleTurn waits for claude to finish writing the turn that just ended.
// Claude reports idle before it flushes the turn's last lines, so the idle
// edge's own reads (the page's chat, the Design check) can miss the final
// message. When the transcript grew since mark, settleTurn redoes the Design
// check and announces the chat again. The session turning busy (cancel) ends
// the wait.
func (s *Server) settleTurn(dir, id string, mark chat.TurnMark, design chat.DesignRef, cancel <-chan struct{}) {
	deadline := time.NewTimer(s.cfg.TurnSettleTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(turnSettlePoll)
	defer tick.Stop()
wait:
	for {
		if ended, err := mark.TurnEnded(); err != nil || ended {
			break
		}
		select {
		case <-cancel:
			return
		case <-s.sessions.Watches.Stop:
			return
		case <-deadline.C:
			break wait
		case <-tick.C:
		}
	}
	info, err := os.Stat(mark.Path)
	if err != nil || info.Size() == mark.Size {
		return
	}
	if design.Conversation != "" {
		s.detectDesignActionable(dir, id, design)
	}
	s.events.Publish(wire.EventSessionChatChanged, id)
}
