package sessions

import "testing"

func TestUpdateSessionMeta_UnreadableMetaIsAnError(t *testing.T) {
	t.Parallel()
	mg := &Manager{Watches: NewSessionWatches()}
	err := mg.UpdateSessionMeta(t.TempDir(), "abcdef", func(*Meta) bool {
		t.Error("change ran on an unreadable meta")
		return false
	})
	if err == nil {
		t.Error("update of an unreadable meta = nil, want an error")
	}
}
