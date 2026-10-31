// Package chatreader reads a conversation file of any harness into one message
// list. It sits above transcript and codexsession because the Codex reader
// returns transcript.ChatMessage.
package chatreader

import (
	"io"
	"os"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/transcript"
)

// Result is one read of a conversation file. Unreadable is set for a Codex
// rollout that has turns but no readable message; CodexVersion is then the
// version that wrote it.
type Result struct {
	Messages     []transcript.ChatMessage
	Unreadable   bool
	CodexVersion string
}

// Read reads a conversation of harness kind from r. Every kind but Codex
// writes the Claude format.
func Read(kind agentrunner.Kind, r io.Reader) (Result, error) {
	if kind == harness.Codex {
		chat, err := codexsession.ReadChat(r)
		if err != nil {
			return Result{}, err
		}
		return Result{Messages: chat.Messages, Unreadable: chat.Unreadable(), CodexVersion: chat.Version}, nil
	}
	msgs, err := transcript.ReadChatFrom(r)
	return Result{Messages: msgs}, err
}

// ReadFile is Read over the file at path. ok is false if the file doesn't
// exist yet.
func ReadFile(kind agentrunner.Kind, path string) (res Result, ok bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, false, nil
		}
		return Result{}, false, err
	}
	defer f.Close()
	res, err = Read(kind, f)
	return res, err == nil, err
}
