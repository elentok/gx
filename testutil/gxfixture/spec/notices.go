package spec

import "github.com/elentok/gx/server/wire"

// NoticesScenario is one Chat session per non-blocking notice kind, each
// carrying only its own notice.
func NoticesScenario() scenario {
	return scenario{
		Chat: true,
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Notices: []string{wire.SessionNoticeUntrustedRead}},
			{Project: project, Prompt: "hello", Notices: []string{wire.SessionNoticeNoComposerMarker}},
		},
	}
}
