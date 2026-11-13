package prompt

import "sync"

// renameSlots holds one pending /rename per chat, in memory only. Every
// request takes the chat's next sequence number and replaces the slot, so a
// queued older send that wakes late finds a newer number and types nothing.
// It is a field of Prompt, with plain methods, to keep Prompt's method count
// under the sizeguard limit.
type renameSlots struct {
	mu    sync.Mutex
	chats map[string]*renameSlot
}

type renameSlot struct {
	seq     uint64
	command string // empty when the request left nothing to type or hold
	// watching is true while a hold watcher serves the chat.
	watching bool
}

// claimWatch reports whether the caller must start the chat's hold watcher.
// One watcher serves every request held for the chat, so a replaced hold
// never leaves two.
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

// endWatch ends the hold watcher and empties the slot if seq is still the
// newest request. It reports false when a newer request arrived, which the
// watcher must then serve.
func (s *renameSlots) endWatch(id string, seq uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := s.chats[id]
	if slot == nil {
		return true
	}
	if slot.seq != seq {
		return false
	}
	slot.command, slot.watching = "", false
	return true
}

// put makes command the chat's pending rename and returns its sequence number.
// An empty command still takes a number: it supersedes an older held one.
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

// owns reports whether seq is still the chat's newest request.
func (s *renameSlots) owns(id string, seq uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := s.chats[id]
	return slot != nil && slot.seq == seq
}

// settle empties the slot's command once the request seq has typed or been
// dropped, unless a newer request replaced it.
func (s *renameSlots) settle(id string, seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot := s.chats[id]; slot != nil && slot.seq == seq {
		slot.command = ""
	}
}

// pending is the chat's held command, empty when none.
func (s *renameSlots) pending(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot := s.chats[id]; slot != nil {
		return slot.command
	}
	return ""
}
