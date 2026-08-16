package subscription

// Severity classifies how a Line should be displayed.
type Severity int

const (
	// SeverityWarning is unmissable but never blocks the run — gx has no
	// control over the account setting itself.
	SeverityWarning Severity = iota
	// SeverityInfo is a quiet, low-emphasis line.
	SeverityInfo
)

// Line is the copy to render for the subscription safety-check banner.
type Line struct {
	Text     string
	Severity Severity
}

const (
	enabledText  = "Your Claude account will auto-purchase extra usage once included usage runs out — a runaway agent loop could incur unexpected charges. Disable this in your account's billing settings."
	disabledText = "Your Claude account will not auto-purchase extra usage."
	unknownText  = "Couldn't verify whether your Claude account auto-purchases extra usage — check your account's billing settings."
)

// BuildLine returns the display line for state, or nil if nothing should be
// shown (the enabled-state warning, once suppressWarning is set).
func BuildLine(state State, suppressWarning bool) *Line {
	switch state {
	case StateEnabled:
		if suppressWarning {
			return nil
		}
		return &Line{Text: enabledText, Severity: SeverityWarning}
	case StateDisabled:
		return &Line{Text: disabledText, Severity: SeverityInfo}
	default:
		return &Line{Text: unknownText, Severity: SeverityInfo}
	}
}
