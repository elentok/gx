package designs_test

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

// Short names for the helpers that live in servertest because the root
// package's tests use them too.
var (
	readMeta      = servertest.ReadMeta
	waitForHolder = servertest.WaitForHolder
	gitOut        = servertest.GitOut
	waitFor       = servertest.Await
	sessionToken  = rigs.SessionToken
)
