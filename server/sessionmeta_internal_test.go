package server

import "testing"

func TestUpdateSessionMeta_UnreadableMetaIsAnError(t *testing.T) {
	t.Parallel()
	s := &Server{sessionWatches: newSessionWatches()}
	err := s.updateSessionMeta(t.TempDir(), "abcdef", func(*sessionMeta) bool {
		t.Error("change ran on an unreadable meta")
		return false
	})
	if err == nil {
		t.Error("update of an unreadable meta = nil, want an error")
	}
}
