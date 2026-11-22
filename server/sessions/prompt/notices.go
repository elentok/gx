package prompt

import (
	"path/filepath"
	"slices"

	"github.com/elentok/gx/server/sessions"
)

// RaiseNotice puts a non-blocking terminal issue on the session's wire data.
// It stays until the next prompt that works without it, or the person's dismiss.
func (p *Prompt) RaiseNotice(dir, kind string) {
	p.sessions.UpdateSessionMetaOrWarn(dir, filepath.Base(dir), func(m *sessions.Meta) bool {
		if slices.Contains(m.Notices, kind) {
			return false
		}
		m.Notices = append(m.Notices, kind)
		return true
	})
}

// ClearNotices drops every notice: the prompt that calls it worked without the
// issues. Notices this prompt raises come after it.
func (p *Prompt) ClearNotices(dir string) {
	p.sessions.UpdateSessionMetaOrWarn(dir, filepath.Base(dir), func(m *sessions.Meta) bool {
		if len(m.Notices) == 0 {
			return false
		}
		m.Notices = nil
		return true
	})
}
