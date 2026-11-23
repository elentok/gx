package renameslot

import "sync"

// Slots holds one pending /rename per chat, in memory only. Every
// request takes the next sequence number, so a late older send types nothing.
type Slots struct {
	mu    sync.Mutex
	chats map[string]*renameSlot
}

type renameSlot struct {
	seq      uint64
	command  string // empty when the request left nothing to type or hold
	watching bool   // a hold watcher serves the chat
}

// ClaimWatch reports whether the caller must start the chat's one hold watcher.
func (s *Slots) ClaimWatch(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := s.chats[id]
	if slot == nil || slot.watching {
		return false
	}
	slot.watching = true
	return true
}

// Current is the chat's newest request: its number and held command.
func (s *Slots) Current(id string) (uint64, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot := s.chats[id]; slot != nil {
		return slot.seq, slot.command
	}
	return 0, ""
}

// EndWatch ends the hold watcher and empties the slot if seq is still newest.
// It reports false when a newer request arrived, which the watcher must serve.
func (s *Slots) EndWatch(id string, seq uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := s.chats[id]
	if slot != nil && slot.seq != seq {
		return false
	}
	if slot != nil {
		slot.command, slot.watching = "", false
	}
	return true
}

// Put makes command the chat's pending rename and returns its sequence number,
// even when empty, so it supersedes an older held one.
func (s *Slots) Put(id, command string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chats == nil {
		s.chats = map[string]*renameSlot{}
	}
	slot := s.chats[id]
	if slot == nil {
		slot = &renameSlot{}
		s.chats[id] = slot
	}
	slot.seq++
	slot.command = command
	return slot.seq
}

func (s *Slots) Forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chats, id)
}

// Owns reports whether seq is still the chat's newest request.
func (s *Slots) Owns(id string, seq uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := s.chats[id]
	return slot != nil && slot.seq == seq
}

// Settle empties the command once request seq typed or dropped, unless a newer one replaced it.
func (s *Slots) Settle(id string, seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot := s.chats[id]; slot != nil && slot.seq == seq {
		slot.command = ""
	}
}

// Has reports whether the chat has a slot.
func (s *Slots) Has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.chats[id]
	return ok
}
