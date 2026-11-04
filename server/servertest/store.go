package servertest

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// WriteConfig writes fields as a config.json at path. The ticket-store path
// is set to store unless fields names one, so a reload sees no startup-only
// change by default. server.watch-config is off unless fields sets it, so a
// command reload never starts a watcher behind a test's back.
func WriteConfig(t *testing.T, path, store string, fields map[string]any) {
	t.Helper()
	all := map[string]any{"ticket-store": map[string]any{"path": store}}
	for k, v := range fields {
		all[k] = v
	}
	srv := map[string]any{"watch-config": false}
	if given, ok := all["server"].(map[string]any); ok {
		maps.Copy(srv, given)
	}
	all["server"] = srv
	data, err := json.Marshal(all)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// WriteTicket creates <store>/<project>/<epic>/issues/<id>-<slug>.md (and the
// project's project.json) with minimal valid frontmatter. Call it before Start.
func WriteTicket(t *testing.T, store, project, epic, id, slug, blockedBy string) {
	t.Helper()
	opts := TicketOpts{}
	if blockedBy != "" {
		opts.BlockedBy = []string{blockedBy}
	}
	WriteTicketWith(t, store, project, epic, id, slug, opts)
}

// TicketOpts are the frontmatter fields WriteTicketWith can set beyond the minimum.
type TicketOpts struct {
	Type      string // default "implement"
	Parent    string
	BlockedBy []string
	Base      string
	Status    string // default "open"
	Agent     string // the ticket's agent: frontmatter field, omitted when empty
	// PauseReason writes status: paused with this pause_reason; Status is ignored.
	PauseReason string
	// ModelRan and ModelExpected are the model_ran and model_expected fields of a
	// model-mismatch pause, omitted when empty.
	ModelRan, ModelExpected string
	// Category is the ticket's category: frontmatter field ("bug", "idea"), omitted when empty.
	Category string
	// ClaimedAt, DoneAt and CancelledAt are RFC 3339 stamps, omitted when empty.
	ClaimedAt, DoneAt, CancelledAt string
	// LongTicketTurns is the turn count the long-ticket alert marked the ticket with; omitted when 0.
	LongTicketTurns int
	// TotalTokens and ActualCost are the ticket's total_tokens and actual_cost
	// stamps; both are omitted when TotalTokens is 0.
	TotalTokens int64
	ActualCost  float64
}

// WriteTicketWith is WriteTicket with a type, a parent and several blockers. Unlike
// WriteTicket it may also run after Start: that is how an agent's turn splits its ticket.
func WriteTicketWith(t *testing.T, store, project, epic, id, slug string, opts TicketOpts) {
	t.Helper()
	if err := WriteTicketFile(store, project, epic, id, slug, opts); err != nil {
		t.Fatal(err)
	}
}

// WriteEpic creates an epic with no tickets, and the project it lives in.
// An empty epic name creates only the project.
func WriteEpic(store, project, epic string) error {
	projectDir := filepath.Join(store, project)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return err
	}
	// Keep an existing project.json: SetProjectRepo's repo must survive a mid-run write.
	if _, err := os.Stat(filepath.Join(projectDir, "project.json")); os.IsNotExist(err) {
		if err := os.WriteFile(filepath.Join(projectDir, "project.json"), []byte(`{"name":"`+project+`"}`), 0o644); err != nil {
			return err
		}
	}
	if epic == "" {
		return nil
	}
	epicDir := filepath.Join(projectDir, epic)
	if err := os.MkdirAll(filepath.Join(epicDir, "issues"), 0o755); err != nil {
		return err
	}
	path := filepath.Join(epicDir, "ticket.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte("---\nstatus: open\n---\n"), 0o644)
}

// WriteTicketFile is WriteTicketWith for a caller with no *testing.T.
func WriteTicketFile(store, project, epic, id, slug string, opts TicketOpts) error {
	if err := WriteEpic(store, project, epic); err != nil {
		return err
	}
	issues := filepath.Join(store, project, epic, "issues")
	if opts.Type == "" {
		opts.Type = "implement"
	}
	if opts.Status == "" {
		opts.Status = "open"
	}
	fm := "---\nid: \"" + id + "\"\nstatus: " + opts.Status + "\ntype: " + opts.Type + "\n"
	if opts.PauseReason != "" {
		fm = "---\nid: \"" + id + "\"\nstatus: paused\npause_reason: " + opts.PauseReason + "\ntype: " + opts.Type + "\n"
	}
	if opts.ModelRan != "" {
		fm += "model_ran: " + opts.ModelRan + "\n"
	}
	if opts.ModelExpected != "" {
		fm += "model_expected: " + opts.ModelExpected + "\n"
	}
	if opts.Parent != "" {
		fm += "parent: \"" + opts.Parent + "\"\n"
	}
	if len(opts.BlockedBy) > 0 {
		fm += "blocked_by: [\"" + strings.Join(opts.BlockedBy, "\", \"") + "\"]\n"
	}
	if opts.Base != "" {
		fm += "base: " + opts.Base + "\n"
	}
	if opts.Agent != "" {
		fm += "agent: " + opts.Agent + "\n"
	}
	if opts.Category != "" {
		fm += "category: " + opts.Category + "\n"
	}
	for _, stamp := range [][2]string{{"claimed_at", opts.ClaimedAt}, {"done_at", opts.DoneAt}, {"cancelled_at", opts.CancelledAt}} {
		if stamp[1] != "" {
			fm += stamp[0] + ": " + stamp[1] + "\n"
		}
	}
	if opts.LongTicketTurns > 0 {
		fm += "long_ticket_turns: " + strconv.Itoa(opts.LongTicketTurns) + "\n"
	}
	if opts.TotalTokens > 0 {
		fm += "total_tokens: " + strconv.FormatInt(opts.TotalTokens, 10) + "\nactual_cost: " + strconv.FormatFloat(opts.ActualCost, 'f', -1, 64) + "\n"
	}
	fm += "---\n\n# " + slug + "\n"
	return os.WriteFile(filepath.Join(issues, id+"-"+slug+".md"), []byte(fm), 0o644)
}
