// Package limits is the verbs that read or flip a server-side limit or mode
// through the API: `gx budget`, `gx usage`, `gx afk` and `gx vimode`.
package limits

import "github.com/elentok/gx/cmd/cmdbase"

// Deps is the runtime context of every command in this package.
type Deps struct {
	cmdbase.Env
}
