package tickets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/tickets/schema"
	"gopkg.in/yaml.v3"
)

var ticketFilenameRe = regexp.MustCompile(`^(\d+)([[:alpha:]]?\d*)-(.+)\.md$`)

// Load reads a `.scratch/` directory from real disk into its epics/tickets.
// A missing directory is not an error: it returns a nil/empty slice, which
// renders the same empty state as a present-but-empty `.scratch/`. Any
// dot-prefixed directory (e.g. `.archive`) is excluded from the result.
func Load(scratchDir string) ([]Epic, error) {
	entries, err := os.ReadDir(scratchDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var epics []Epic
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		epics = append(epics, loadEpic(scratchDir, entry.Name()))
	}
	return epics, nil
}

func loadEpic(scratchDir, name string) Epic {
	epicPath := filepath.Join(scratchDir, name)
	epic := Epic{Name: name, Path: epicPath}

	if raw, err := os.ReadFile(filepath.Join(epicPath, "map.md")); err == nil {
		epic.IsMap = true
		epic.MapBody = string(raw)
	}

	loadEpicTicketMD(&epic)

	issuesDir := filepath.Join(epicPath, "issues")
	issueEntries, err := os.ReadDir(issuesDir)
	if err != nil {
		return epic
	}

	for _, issueEntry := range issueEntries {
		if issueEntry.IsDir() {
			continue
		}
		number, identifier, title, ok := parseTicketFilename(issueEntry.Name())
		if !ok {
			continue
		}

		ticketPath := filepath.Join(issuesDir, issueEntry.Name())
		ticket := Ticket{
			Number:     number,
			Identifier: identifier,
			Title:      title,
			Path:       ticketPath,
		}

		raw, err := os.ReadFile(ticketPath)
		if err != nil {
			ticket.ReadErr = err.Error()
			epic.Tickets = append(epic.Tickets, ticket)
			continue
		}

		parsed, err := schema.ParseTicketFromRaw(string(raw), ticketPath)
		if err != nil {
			ticket.ReadErr = err.Error()
			epic.Tickets = append(epic.Tickets, ticket)
			continue
		}

		ticket.Type = string(parsed.Type)
		ticket.BlockedBy = idsToStrings(parsed.BlockedBy)
		ticket.Parent = idToStringPtr(parsed.Parent)
		ticket.Status = string(parsed.Status)
		ticket.Body = schema.ParseBody(string(raw))
		ticket.ActualContextWindow = parsed.ActualContextWindow
		ticket.ExpectedContextWindow = parsed.ExpectedContextWindow
		ticket.ElapsedTime = parsed.ElapsedTime
		ticket.ActualCost = parsed.ActualCost
		ticket.Compactions = parsed.Compactions
		ticket.Commitless = parsed.IsCommitless()
		ticket.Notify = parsed.Notify
		ticket.NoRecover = parsed.NoRecover
		ticket.Unique = parsed.Unique
		ticket.ParkKind = parsed.ParkKind
		ticket.Mutes = parsed.Mutes
		ticket.Base = parsed.Base
		ticket.ResolvedBase = parsed.ResolvedBase
		epic.Tickets = append(epic.Tickets, ticket)
	}

	epic.quarantineInvalidParents()
	epic.flagDanglingBlockers()

	return epic
}

// parseTicketFilename splits a "NN[suffix]-<slug>.md" filename into its
// numeric sort key, full identifier, and a humanized title. The optional
// suffix supports wayfinder's split tickets (for example 10a) and, one
// level deeper, a numeric child of a lettered split (10b1) — see
// tickets.NextTicketID.
func parseTicketFilename(filename string) (number int, identifier, title string, ok bool) {
	m := ticketFilenameRe.FindStringSubmatch(filename)
	if m == nil {
		return 0, "", "", false
	}
	number, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, "", "", false
	}
	identifier = m[1] + m[2]
	return number, identifier, humanizeSlug(m[3]), true
}

func humanizeSlug(slug string) string {
	title := strings.ReplaceAll(slug, "-", " ")
	if title == "" {
		return title
	}
	return strings.ToUpper(title[:1]) + title[1:]
}

// ticketMDYAML is the frontmatter of a top-level ticket's `ticket.md`.
type ticketMDYAML struct {
	Kind        string     `yaml:"kind"`
	Status      string     `yaml:"status"`
	BlockedBy   []string   `yaml:"blocked_by"`
	Base        string     `yaml:"base"`
	StartedAt   *time.Time `yaml:"started_at"`
	CompletedAt *time.Time `yaml:"completed_at"`
}

// WriteEpicTicketMD writes a minimal ticket.md for the epic at epicPath, with
// the given status and the directory name as its heading. An existing
// ticket.md is left alone: it carries the epic's own fields and body.
func WriteEpicTicketMD(epicPath string, status schema.Status) error {
	f, err := os.OpenFile(filepath.Join(epicPath, "ticket.md"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "---\nstatus: %s\n---\n\n# %s\n", status, filepath.Base(epicPath))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// loadEpicTicketMD fills epic from its ticket.md frontmatter. A missing
// ticket.md leaves HasTicketMD false, which ValidateProject reports.
func loadEpicTicketMD(epic *Epic) {
	raw, err := os.ReadFile(filepath.Join(epic.Path, "ticket.md"))
	if err != nil {
		return
	}
	epic.HasTicketMD = true

	fm, ok := schema.FrontmatterYAML(string(raw))
	if !ok {
		return
	}
	var wire ticketMDYAML
	if err := yaml.Unmarshal([]byte(fm), &wire); err != nil {
		return
	}
	if wire.Kind == KindMap {
		epic.IsMap = true
		epic.MapBody = schema.ParseBody(string(raw))
	}
	epic.Status = wire.Status
	epic.BlockedBy = wire.BlockedBy
	epic.Base = wire.Base
	if wire.StartedAt != nil {
		epic.StartedAt = *wire.StartedAt
	}
	if wire.CompletedAt != nil {
		epic.CompletedAt = *wire.CompletedAt
	}
}

// epicTiming is an epic's optional started_at/completed_at pair, as stored in
// ticket.md's frontmatter (and in migrate's old-shape epic.yaml sidecar).
type epicTiming struct {
	StartedAt   *time.Time `yaml:"started_at,omitempty"`
	CompletedAt *time.Time `yaml:"completed_at,omitempty"`
}

// StampEpicStarted writes started_at into scratchDir/epicName's ticket.md
// frontmatter. It is idempotent:
// if started_at is already set, it leaves the file untouched, so calling it
// on every ticket claim — not just the epic's first — never overwrites an
// already-recorded start time across a resumed or reattached run.
func StampEpicStarted(scratchDir, epicName string, now time.Time) error {
	return stampEpicTiming(scratchDir, epicName, func(wire *epicTiming) bool {
		if wire.StartedAt != nil {
			return false
		}
		wire.StartedAt = &now
		return true
	})
}

// StampEpicCompleted writes completed_at into scratchDir/epicName's
// ticket.md frontmatter. It is idempotent the same way StampEpicStarted is: a
// completed_at already on disk is left alone, so completion is only ever
// recorded once, at genuine completion.
func StampEpicCompleted(scratchDir, epicName string, now time.Time) error {
	return stampEpicTiming(scratchDir, epicName, func(wire *epicTiming) bool {
		if wire.CompletedAt != nil {
			return false
		}
		wire.CompletedAt = &now
		return true
	})
}

// stampEpicTiming reads scratchDir/epicName's ticket.md timing, hands it to
// mutate, and writes it back only when mutate reports a change — the shared
// idempotency check both StampEpicStarted and StampEpicCompleted rely on.
func stampEpicTiming(scratchDir, epicName string, mutate func(*epicTiming) bool) error {
	epicPath := filepath.Join(scratchDir, epicName)
	ticketPath := filepath.Join(epicPath, "ticket.md")
	if _, err := os.Stat(ticketPath); err != nil {
		return fmt.Errorf("epic %s has no ticket.md: %w", epicPath, err)
	}
	// The lock spans the read inside stampTicketMDTiming too, or a
	// concurrent `set`/`section` write between read and write is lost.
	unlock, err := schema.LockTicket(ticketPath)
	if err != nil {
		return err
	}
	defer unlock()
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		return err
	}
	return stampTicketMDTiming(ticketPath, string(raw), mutate)
}

// stampTicketMDTiming applies mutate to the timing fields of ticket.md's
// frontmatter. It edits the YAML node tree rather than re-marshaling a struct,
// so the epic's other frontmatter keys and its body survive untouched.
func stampTicketMDTiming(path, raw string, mutate func(*epicTiming) bool) error {
	fm, body, ok := schema.SplitFrontmatter(raw)
	if !ok {
		return fmt.Errorf("%s: no frontmatter to stamp timing into", path)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s: frontmatter is not a mapping", path)
	}
	var wire epicTiming
	if err := doc.Content[0].Decode(&wire); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	before := wire
	if !mutate(&wire) {
		return nil
	}
	if wire.StartedAt != before.StartedAt {
		setMappingTime(doc.Content[0], "started_at", *wire.StartedAt)
	}
	if wire.CompletedAt != before.CompletedAt {
		setMappingTime(doc.Content[0], "completed_at", *wire.CompletedAt)
	}
	out, err := yaml.Marshal(doc.Content[0])
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", path, err)
	}
	return writeFileAtomic(path, []byte("---\n"+string(out)+"---\n"+body))
}

// setMappingTime sets key to t in mapping, appending the key when absent.
func setMappingTime(mapping *yaml.Node, key string, t time.Time) {
	var val yaml.Node
	_ = val.Encode(t)
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = &val
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &val)
}

// writeFileAtomic replaces path's content via a same-directory temp file
// plus rename, so a concurrent reader never observes a torn/truncated write.
// Duplicated from ralphloop's/schema's writeFileAtomic (a ~15-line helper)
// rather than exported cross-package, per
// .scratch/ralph-tickets-visibility/issues/02-tickets-set-cli.md's Answer.
// Shared within this package by both ticket.md timing writes and
// tickets/migrate.go's ticket-file rewrites.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0644); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// idsToStrings lowers schema.TicketIDs to plain strings for tickets.Ticket's
// BlockedBy field, which predates the schema package and is read directly by
// ralphloop/ui call sites that don't know about schema.TicketID.
func idsToStrings(ids []schema.TicketID) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

// idToStringPtr lowers an optional schema.TicketID to a plain *string, for
// tickets.Ticket's Parent field (see idsToStrings).
func idToStringPtr(id *schema.TicketID) *string {
	if id == nil {
		return nil
	}
	s := string(*id)
	return &s
}
