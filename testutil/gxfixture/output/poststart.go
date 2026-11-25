package output

import "github.com/elentok/gx/server/wire"

// PostStartEnv is what a scenario's PostStart hook works with once the server is up.
type PostStartEnv struct {
	Root, Repo, Store string
	// Git runs git in dir; a non-zero exit is an error.
	Git func(dir string, args ...string) error
	// Sessions lists the server's sessions.
	Sessions func() ([]wire.SessionInfo, error)
}
