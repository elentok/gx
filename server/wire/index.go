package wire

import (
	"time"

	"github.com/elentok/gx/tickets/schema"
)

// TicketInfo is one ticket in the snapshot. Markdown stays the truth: this is
// a read-only projection rebuilt from the store scan.
type TicketInfo struct {
	Address string `json:"address"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	// RenderedStatus is tickets.Epic.RenderedStatus's word (blocked,
	// waiting-for-children, ...): the one status every client shows.
	RenderedStatus string   `json:"rendered_status"`
	Type           string   `json:"type"`
	BlockedBy      []string `json:"blocked_by,omitempty"`
	// ExternalBlockers and WidenedBlockers carry the cross-epic resolution
	// (see tickets.Ticket), which a client can't redo from one scope's tickets.
	ExternalBlockers []string          `json:"external_blockers,omitempty"`
	WidenedBlockers  map[string]string `json:"widened_blockers,omitempty"`
	// Parent is the address of the ticket this one was forked from.
	Parent string `json:"parent,omitempty"`
	// File is the ticket's markdown path, for clients that edit content or
	// set a plain status directly (those are direct writes, not server ones).
	File string `json:"file,omitempty"`
	// ClaimedAt is when the server launched the claimed ticket's iteration;
	// filled in by the snapshot handler, zero while the ticket is not running.
	ClaimedAt time.Time `json:"claimed_at,omitzero"`
	// LaunchStep is the launch step of the claimed ticket's run; zero once the
	// launch ended.
	LaunchStep LaunchStep `json:"launch_step,omitzero"`
	// Recovery is RecoveryPending or RecoveryEscalated for a paused ticket that
	// recovery is working on or gave up on; filled in by the snapshot handler.
	Recovery string `json:"recovery,omitempty"`
	// The merging-time metrics the Queue header and the preview sum up; zero
	// until the ticket merges.
	ActualContextWindow   int     `json:"actual_context_window,omitempty"`
	ExpectedContextWindow int     `json:"expected_context_window,omitempty"`
	ElapsedTime           int     `json:"elapsed_time,omitempty"`
	ActualCost            float64 `json:"actual_cost,omitempty"`
	HadCodex              bool    `json:"had_codex,omitempty"`
	Compactions           int     `json:"compactions,omitempty"`

	// HarnessLabel is filled by the snapshot handler for open, blocked and claimed tickets.
	HarnessLabel `tstype:",extends"`
	Category     string    `json:"category,omitempty"`
	ExportedAt   time.Time `json:"exported_at,omitzero"`
	// Mutes is what the Context menu's "Unmute & Reopen" keys off.
	Mutes []schema.MuteRecord `json:"mutes,omitempty"`

	// EpicMergedAt is the ticket's epic's merged_at. It rides on every ticket
	// rather than in a separate epic list so a stamp reaches clients through
	// the same ticket-changed events as any other edit.
	EpicMergedAt time.Time `json:"epic_merged_at,omitzero"`
	// EpicIsMap is set on a wayfinder map epic's tickets, riding along like
	// EpicMergedAt.
	EpicIsMap bool `json:"epic_is_map,omitempty"`
	// EpicStatus is the epic's ticket.md status, riding along likewise.
	EpicStatus string `json:"epic_status,omitempty"`
	// EpicMergeTarget is the branch an epic merge merges on; filled in by the
	// snapshot handler, empty when the project has no repo or trunk.
	EpicMergeTarget string `json:"epic_merge_target,omitempty"`
}

// MigrationFailure is a saved failed migration as the live state reports it.
type MigrationFailure struct {
	Project string `json:"project"`
	Error   string `json:"error"`
	// RetryCommand is the shell command that migrates the project again.
	RetryCommand string `json:"retry_command"`
}

// Snapshot is the /v1/snapshot payload: tickets sorted by address (so each
// epic's fork tree reads top-down via Parent) and the sequence number of the
// index state they were read at.
type Snapshot struct {
	Seq uint64 `json:"seq"`
	// Build is the server's own version, filled in by the handler. Empty from a
	// server that predates it.
	Build string `json:"build,omitempty"`
	// Instance is new on every server start; a restart is done when it changes.
	// Filled in by the handler.
	Instance string `json:"instance,omitempty"`
	// MigrationFailures are the saved failed migrations; filled in by the
	// handler, so a change to the record shows without a restart.
	MigrationFailures []MigrationFailure `json:"migration_failures,omitempty"`
	// HerdrUnavailable is set while herdr isn't answering and a ticket needs it;
	// the server's own view, filled in by the handler rather than the index.
	// HerdrText says which tickets wait.
	HerdrUnavailable bool   `json:"herdr_unavailable,omitempty"`
	HerdrText        string `json:"herdr_text,omitempty"`
	// Migrating is set until the start-up migration run ends; filled in by the handler.
	Migrating bool `json:"migrating,omitempty"`
	// Budget is today's spend, filled in by the handler like HerdrUnavailable.
	Budget BudgetStatus `json:"budget"`
	// ExtraUsage is set while the account auto-purchases extra usage, so the
	// enqueue confirm can warn; filled in by the handler.
	ExtraUsage bool `json:"extra_usage,omitempty"`
	// Disks lists the disks under the warn threshold; filled in by the handler.
	Disks []DiskStatus `json:"disks,omitempty"`
	// Health is the CPU, memory and disk meters; filled in by the handler.
	Health  Health       `json:"health"`
	Tickets []TicketInfo `json:"tickets"`
	// Pending is every queue entry with its explain verdict, so a client never
	// asks per row. Filled in by the handler, like HerdrUnavailable.
	Pending []PendingRow `json:"pending"`
	// Mode is QueueModeRunning or QueueModePaused; filled in by the handler.
	// Empty from a server that predates it.
	Mode QueueMode `json:"mode,omitempty"`
	// AFK is set while AFK mode sends session banners to chat too; filled in by
	// the handler.
	AFK bool `json:"afk,omitempty"`
	// ViMode is set while the web UI's vi mode is on; filled in by the handler.
	// Missing means off.
	ViMode bool `json:"vi_mode,omitempty"`
	// SkillsInstallProblem is why the skill install `gx server start` ran
	// last did not finish; empty when it did. Filled in by the handler.
	SkillsInstallProblem string `json:"skills_install_problem,omitempty"`
	// NodeMissing is set while node is not on the server's PATH, so web
	// dependency installs fail; filled in by the handler.
	NodeMissing bool `json:"node_missing,omitempty"`
	// Sessions are the interactive sessions, oldest first; filled in by the
	// handler.
	Sessions []SessionInfo `json:"sessions"`
	// Epics summarizes every epic holding a queue item; filled in by the handler.
	Epics []EpicSummary `json:"epics"`
	// RecentlyFinished is the last completed loop epics, newest first; filled in
	// by the handler.
	RecentlyFinished []FinishedEpic `json:"recently_finished"`
}
