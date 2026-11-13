package epicflows_test

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
)

var (
	repairToken           = rigs.RepairToken
	readMeta              = servertest.ReadMeta
	report                = servertest.Report
	waitForSession        = servertest.WaitForSession
	writeAssistantMessage = rigs.WriteAssistantMessage
)
