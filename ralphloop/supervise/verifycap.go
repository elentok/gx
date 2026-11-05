package supervise

// VerifyCapState is the iteration-wide "cap already fired" state, shared by
// every wait of one iteration so a later wait (the corrective retry) does not
// stop the session a second time.
type VerifyCapState struct {
	Fired bool
}
