//go:build gxfixture

package harness

// NoChat is the fixture-only harness name with no chat reader.
const NoChat = "nochat"

// nochatHarness runs the fakeclaude on PATH like claude, but settings.HasChatReader
// is false for its name, so the server treats its sessions as NoChat. Only the
// gxfixture build registers it, so a release binary has no such harness.
type nochatHarness struct{ claudeHarness }

func init() {
	Register(NoChat, nochatHarness{})
}
