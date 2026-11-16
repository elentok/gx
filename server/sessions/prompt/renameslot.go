package prompt

import "sync"

// renameSlots holds one pending /rename per chat, in memory only. Every
// request takes the next sequence number, so a late older send types nothing.
type renameSlots struct {
	mu    sync.Mutex
	chats map[string]*renameSlot
}

type renameSlot struct {
	seq      uint64
	command  string // empty when the request left nothing to type or hold
	watching bool   // a hold watcher serves the chat
}

// claimWatch reports whether the caller must start the chat's one hold watcher.
func (s *renameSlots) claimWatch(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := s.chats[id]
	if slot == nil || slot.watching {
		return false
	}
	slot.watching = true
	return true
}

// current is the chat's newest request: its number and held command.
func (s *renameSlots) current(id string) (uint64, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot := s.chats[id]; slot != nil {
		return slot.seq, slot.command
	}
	return 0, ""
}

// endWatch ends the hold watcher and empties the slot if seq is still newest.
// It reports false when a newer request arrived, which the watcher must serve.
func (s *renameSlots) endWatch(id string, seq uint64) bool {
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

// put makes command the chat's pending rename and returns its sequence number,
// even when empty, so it supersedes an older held one.
func (s *renameSlots) put(id, command string) uint64 {
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

func (s *renameSlots) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.chats, id)
}

// owns reports whether seq is still the chat's newest request.
func (s *renameSlots) owns(id string, seq uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := s.chats[id]
	return slot != nil && slot.seq == seq
}

// settle empties the command once request seq typed or dropped, unless a newer one replaced it.
func (s *renameSlots) settle(id string, seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot := s.chats[id]; slot != nil && slot.seq == seq {
		slot.command = ""
	}
}
