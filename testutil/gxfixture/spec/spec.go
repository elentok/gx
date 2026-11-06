// Package spec holds the data model of the gxfixture scenarios and the scenario
// definitions that need nothing from the fixture program itself.
package spec

import (
	"slices"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/limitseed"
)

const project = limitseed.Project

// Short names for the definition files in this package.
type (
	ticket         = Ticket
	epic           = Epic
	scenario       = Scenario
	recommendation = Recommendation
	sessionSpec    = SessionSpec
	commandStep    = CommandStep
)

// Ticket is one seeded ticket of an epic.
type Ticket struct {
	ID, Slug string
	Opts     servertest.TicketOpts
}

// Epic is one seeded epic.
type Epic struct {
	Project, Name string
	Tickets       []Ticket
}

// Git runs git in dir for a GitSetup hook.
type Git func(dir string, args ...string) error

// Scenario is what one gxfixture scenario seeds and switches on.
type Scenario struct {
	// Projects have no epics.
	Projects []string
	Epics    []Epic
	// Behaviour is the fake worker's per-ticket behaviour, by address.
	Behaviour map[string]string
	// RateLimitFromUsage makes the ratelimit worker's reset come from a usage window that
	// resets soon, and shortens the hold's wait past that reset, so a flow can wait the hold out.
	RateLimitFromUsage bool
	// PromptGrace, when set, is the server's wait for a typed key or prompt to take effect
	// (GX_PROMPT_GRACE); empty means the 45s default.
	PromptGrace string
	// DesignSteps makes each epic, by address, a map and sets its design step
	// with the CLI verb, so no spec file is needed in trunk.
	DesignSteps map[string]string
	// Design scripts the design agent's turns (agentfake.DesignTurn*), one per
	// prompt in order; none means the fake's default.
	Design []string
	// DesignByMap scripts the design turns of one map, by address, over Design.
	DesignByMap map[string][]string
	// Queue lists the epics put on the queue once the server is up.
	Queue []string
	// QueueAgents is the agent a queued address runs under (`queue add --agent`);
	// an address missing here is queued with no agent, so it follows the default.
	QueueAgents map[string]string
	// RanOnClaude lists tickets that get a real Claude run before the queue loop:
	// the fake asks, the ticket is sent back to waiting, and it keeps its recorded
	// session. Their epics are queued by the queue loop like any other.
	RanOnClaude []string
	// Transcripts makes the fake Claude record a ticket prompt in its transcript.
	Transcripts bool
	// Config is --config style key=value overrides of the global config.
	Config []string
	// EpicMeta replaces an epic's ticket.md frontmatter (the lines between the
	// "---" fences), by address, e.g. "status: draft\n".
	EpicMeta map[string]string
	// Recovered seeds one applied, successful recovery in an epic's run log,
	// by epic address, with the failure kind it recovered (an events.Kind). A
	// later failure of the same kind in that epic is then not recovered again.
	Recovered map[string]string
	// ManualMerge lists the projects whose auto-ff-merge is off; the others have it on.
	ManualMerge []string
	// ProjectExtra is raw project.json members (leading comma) added per project.
	ProjectExtra map[string]string
	// Hold keeps every worker running until the hold file is removed, as --hold does.
	Hold bool
	// AgentProcs writes the first fake agent's pids into the process table once it runs.
	AgentProcs bool
	// RecentlyFinished is the server.recently-finished cap; zero keeps the default.
	RecentlyFinished int
	// OpenDesigns lists the maps whose design session is opened once the server is up.
	OpenDesigns []string
	// AutoAdvanceOff lists the opened maps whose design session gets auto-advance off.
	AutoAdvanceOff []string
	// AdvanceSteps moves each map, by address, to a design step once its design
	// session is open, as the agent does while it charts.
	AdvanceSteps map[string]string
	// LinkedDesigns lists the maps that get an extra plain session, linked to the
	// map as a design session before OpenDesigns runs, so it is the older run.
	LinkedDesigns []string
	// Conversations seeds the conversation records, and the time recording began
	// (the "started" marker). It is called with the time of seeding, so the days
	// it picks stay inside the stats range.
	Conversations func(now time.Time) (started time.Time, recs []conversations.Record)
	// Recommend stores an investigate recommendation on the open round of a
	// paused ticket, by ticket address.
	Recommend map[string]Recommendation
	// GitSetup shapes the project repo (branches, a dirty checkout) after its first commit.
	GitSetup func(git Git, repo string) error
	// NoCheckConfig leaves the projects without a checks key in project.json; the
	// other scenarios write an empty one, so the Epics page shows no banner.
	NoCheckConfig bool
	// OwnRepos lists the projects that get a repo of their own instead of the shared one.
	OwnRepos []string
	// QueueAfterSessions queues sc.Queue once the sessions are seeded: a Queue
	// agent's control file would otherwise break seedSession's before/after diff.
	QueueAfterSessions bool
	// QueueAfterUsage lists the epics queued once the server has stored a usage
	// sample, so the usage gate is already up when they first try to claim.
	QueueAfterUsage []string
	// Usage makes the fake workers report the weekly window at 99%.
	Usage bool
	// CodexUsage makes the fake Codex workers write a weekly sample at 99% into their rollout.
	CodexUsage bool
	// DiskLow gives each project a cleanup command that follows the disk's clean file.
	DiskLow bool
	// FakeGo writes the gate dir's fake go (see cachegate.go).
	FakeGo bool
	// FakeNpm writes the gate dir's fake npm and holds it (see launchstate.go).
	FakeNpm bool
	// FailingNpm makes that fake npm fail at once instead of holding.
	FailingNpm bool
	// DropAskedAgent forces the server to clean up an asking agent, as a
	// herdr-hosted run does (see keepworktree.go).
	DropAskedAgent bool
	// UnknownSkill makes the fake TUI agent reject every skill prompt as an unknown command.
	UnknownSkill bool
	// Env is extra server environment, for the fake's knobs and the server's test hooks.
	Env []string
	// Restart points the server's restart at a fake gx binary (see restart.go).
	Restart *RestartSpec
	// Chat turns the fake's chat knob on, so a plain prompt is a chat turn.
	Chat bool
	// ChatCapture points the server's Slack webhook and Telegram base URL at a
	// capture endpoint and prints the file it appends to (see chat.go).
	ChatCapture bool
	// Steps are gx argument lists run against the server once the queue is set,
	// in order; the first failing one stops the fixture.
	Steps [][]string
	// Sessions are created in order once the server is up, as `gx agents create` makes them.
	Sessions []SessionSpec
	// Commands is what the fakes' list probes answer; nil leaves them empty.
	Commands *agentfake.ProbeConfig
	// Codex installs the fake codex on PATH without the --fakecodex flag, for a
	// scenario that runs a Codex session.
	Codex bool
	// StallThreshold is how long a Working session's transcript may stay untouched
	// before it counts as stalled; the --stall-threshold flag overrides it.
	StallThreshold string
	// Web is how far the fixture takes the web login; the zero value signs in.
	Web WebMode
	// ExtraLogins seeds web logins before the server starts, as how long ago each was last used.
	ExtraLogins []time.Duration
	// Limits seeds the limits state file before the server starts, in the
	// shape the --shape flag names (default fresh).
	Limits bool
	// Spend seeds the budget ledger before the server starts, in the shape the
	// --shape flag names (default normal), and prints the held agent's transcript path.
	Spend bool
	// CheckWaiting writes a live check-lock waiting state for the held agent's
	// worktree once it runs, and prints the file's path.
	CheckWaiting bool
	// LimitBurn seeds the Claude limits file and the limit history before the
	// server starts, in the shape the --shape flag names (default menu).
	LimitBurn bool
	// TokenPoll shortens the token ledger poll and prints the four live
	// transcripts (the limit-burn live shape).
	TokenPoll bool
	// Migrate turns on the migration test hooks and the bare-server control script (see migrate.go).
	Migrate bool
	// MigrateHold also holds the start-up migration open until the hold file is removed.
	MigrateHold bool
}

// WebMode is how far the fixture takes the web setup and sign-in.
type WebMode int

const (
	// WebSignedIn finishes the setup and keeps the login cookie.
	WebSignedIn WebMode = iota
	// WebSetupPending leaves the setup undone.
	WebSetupPending
	// WebSignedOut finishes the setup and drops the login cookie.
	WebSignedOut
)

// ProjectNames is every project the scenario creates, in first-seen order.
func (sc Scenario) ProjectNames() []string {
	names := slices.Clone(sc.Projects)
	for _, e := range sc.Epics {
		if !slices.Contains(names, e.Project) {
			names = append(names, e.Project)
		}
	}
	return names
}

// Recommendation is an investigate recommendation stored on a paused ticket.
type Recommendation struct{ Verb, Diagnosis string }

// SessionSpec is one interactive session a scenario creates once the server is
// up, in order. A prompt makes a chat turn (the scenario turns the fake's chat
// knob on); with the agentfake.ChatSlowMarker the turn never ends.
type SessionSpec struct {
	Project string
	// Harness is empty for Claude.
	Harness string
	Prompt  string
	// Link is the map address the finished session is marked a design session of.
	Link string
	// Read clears the Unread the finished turn (or the crash) raised.
	Read bool
	// Actionable is the reason the session raises with `gx agent actionable`.
	Actionable string
	// Crash makes the fake exit with code 1 once the session is idle.
	Crash bool
	// Control is sent to the fake once the session settled; Failure is the
	// Failure.Kind the session must then show.
	Control *agentfake.Control
	Failure string
	// ThenPrompt is one more chat turn sent after the control, so a reply
	// carries what the control changed.
	ThenPrompt string
	// Steps are "/" commands sent once the session is idle, in order.
	Steps []CommandStep
	// Images appends the saved-image messages of agentsImagesScenario to the chat.
	Images bool
	// Rejected says the agent rejects the first prompt (see Scenario.UnknownSkill):
	// the session is settled once it raises the Unknown command Actionable.
	Rejected bool
}

// CommandStep is one "/" command a seeded session runs. Output is the stdout
// the Claude fake writes for it (empty for a command with no output); nil
// sends no control, as for Codex, which writes no output. Turn sends a command
// the fake treats as a chat turn (/compact) and waits for its reply.
type CommandStep struct {
	Prompt string
	Output *string
	Turn   bool
	// Control is sent before the prompt. Blocks says the command leaves the
	// session waiting at a dialog, so the step does not wait for it to idle.
	Control *agentfake.Control
	Blocks  bool
}

// RestartPath is where the fake restart binary sits.
type RestartPath int

const (
	// RestartAtDefault is <root>/restart/bin/gx.
	RestartAtDefault RestartPath = iota
	// RestartAtLaunchd is <root>/launchd/bin/gx, as an installed launchd agent would name it.
	RestartAtLaunchd
	// RestartMissing names a path with no file behind it.
	RestartMissing
)

// DefaultNewVersion is the version the fake restart binary reports unless the spec sets one.
const DefaultNewVersion = "fixture-next"

// RestartSpec is a scenario's fake restart binary.
type RestartSpec struct {
	Path RestartPath
	// Version is what the fake reports as the new version.
	Version string
	// ConfigError makes the config check fail with this message.
	ConfigError string
	// Failing maps a project to its dry-run error; the other projects pass.
	Failing map[string]string
	// Pending maps a project to its pending step count; the others report 0.
	Pending map[string]int
	// Old makes the binary reject restart-check, as a binary older than the command does.
	Old bool
	// Slow is how long restart-check takes, in seconds ("0.5").
	Slow string
	// StartsUp lets the restart bring the server back.
	StartsUp bool
}

// CheckOutput is what the fake prints for the scenario's projects.
func (r RestartSpec) CheckOutput(projects []string) wire.RestartCheckOutput {
	version := r.Version
	if version == "" {
		version = DefaultNewVersion
	}
	out := wire.RestartCheckOutput{Version: version, Config: wire.RestartCheckConfig{OK: r.ConfigError == "", Error: r.ConfigError}}
	for _, name := range projects {
		p := wire.RestartCheckProject{Name: name, OK: true, Files: []string{}, Pending: r.Pending[name]}
		if msg, bad := r.Failing[name]; bad {
			p.OK, p.Error = false, msg
		}
		out.Projects = append(out.Projects, p)
	}
	return out
}
