package spec

// DevModeShapes are the --shape values of the dev-mode scenario, the first
// being the default. Each is a state of the snapshot's Dev block; the scenario
// is Scenario.DevMode and its setup is the fixture program's.
var DevModeShapes = []string{"running", "off", "installing", "starting", "down", "no-checkout"}
