package spec

import "github.com/elentok/gx/testutil/agentfake"

// VimLaunch: the fake Claude shows a vim INSERT footer on every launch, ignoring
// the editor-mode override, so each launch logs the vim probe lines. The queued
// ticket still finishes (the prompt is typed anyway) and a web session launches
// too.
var VimLaunch = func() scenario {
	sc := withEnv(stallEpic("vim-launch-epic", agentfake.BehaviourFinish), agentfake.TUIIgnoreEditorModeEnv+"=1")
	sc.Chat = true
	sc.Sessions = []sessionSpec{{Project: project, Prompt: "hello"}}
	return sc
}()
