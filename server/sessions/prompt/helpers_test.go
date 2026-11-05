package prompt_test

import "github.com/elentok/gx/server/servertest"

// Aliases for the helpers that live in servertest, so the callers here keep
// their short names.
const (
	runEndText     = servertest.RunEndText
	countdownDelay = servertest.CountdownDelay
)

type grillingFixture = servertest.GrillingFixture

var (
	readMeta              = servertest.ReadMeta
	report                = servertest.Report
	screenHas             = servertest.ScreenHas
	waitForHolder         = servertest.WaitForHolder
	waitForScreen         = servertest.WaitForScreen
	waitForSession        = servertest.WaitForSession
	waitUntil             = servertest.WaitUntil
	storeWithMapStep      = servertest.StoreWithMapStep
	writeAssistantMessage = servertest.WriteAssistantMessage
	clearByInput          = servertest.ClearByInput
	startAdvancing        = servertest.StartAdvancing
	endTurn               = servertest.EndTurn
	sessionSnapshot       = servertest.SessionSnapshot
	autoSends             = servertest.AutoSends
	startWebDesign        = servertest.StartWebDesign
	webSessionWrite       = servertest.WebSessionWrite
	sessionToken          = servertest.SessionToken
)
