package tickets

import (
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

	epic.StartedAt, epic.CompletedAt = loadEpicTiming(epicPath)

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
		ticket.Children = idsToStrings(parsed.Children)
		ticket.Parent = idToStringPtr(parsed.Parent)
		ticket.Status = string(parsed.Status)
		ticket.Body = schema.ParseBody(string(raw))
		ticket.ActualContextWindow = parsed.ActualContextWindow
		ticket.ElapsedTime = parsed.ElapsedTime
		ticket.Compactions = parsed.Compactions
		ticket.Commitless = parsed.IsCommitless()
		epic.Tickets = append(epic.Tickets, ticket)
	}

	epic.quarantineInvalidParents()

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

// epicYAML is the on-disk shape of an epic's optional `epic.yaml` sidecar
// file (see loadEpicTiming). Both fields are optional: an epic with no
// epic.yaml, or one that omits a field, leaves the corresponding Epic
// timestamp zero-valued rather than erroring.
type epicYAML struct {
	StartedAt   *time.Time `yaml:"started_at,omitempty"`
	CompletedAt *time.Time `yaml:"completed_at,omitempty"`
}

// loadEpicTiming reads epicPath's epic.yaml sidecar, if present, for the
// epic's started_at/completed_at timestamps. A missing or unparsable file is
// not an error here — it just leaves both timestamps zero, the same as an
// epic with no timing data recorded yet.
func loadEpicTiming(epicPath string) (startedAt, completedAt time.Time) {
	raw, err := os.ReadFile(filepath.Join(epicPath, "epic.yaml"))
	if err != nil {
		return time.Time{}, time.Time{}
	}

	var wire epicYAML
	if err := yaml.Unmarshal(raw, &wire); err != nil {
		return time.Time{}, time.Time{}
	}

	if wire.StartedAt != nil {
		startedAt = *wire.StartedAt
	}
	if wire.CompletedAt != nil {
		completedAt = *wire.CompletedAt
	}
	return startedAt, completedAt
}

// StampEpicStarted writes started_at into scratchDir/epicName's epic.yaml
// sidecar, creating both the directory and file if needed. It is idempotent:
// if started_at is already set, it leaves the file untouched, so calling it
// on every ticket claim — not just the epic's first — never overwrites an
// already-recorded start time across a resumed or reattached run.
func StampEpicStarted(scratchDir, epicName string, now time.Time) error {
	return stampEpicTiming(scratchDir, epicName, func(wire *epicYAML) bool {
		if wire.StartedAt != nil {
			return false
		}
		wire.StartedAt = &now
		return true
	})
}

// StampEpicCompleted writes completed_at into scratchDir/epicName's
// epic.yaml sidecar. It is idempotent the same way StampEpicStarted is: a
// completed_at already on disk is left alone, so completion is only ever
// recorded once, at genuine completion.
func StampEpicCompleted(scratchDir, epicName string, now time.Time) error {
	return stampEpicTiming(scratchDir, epicName, func(wire *epicYAML) bool {
		if wire.CompletedAt != nil {
			return false
		}
		wire.CompletedAt = &now
		return true
	})
}

// stampEpicTiming reads scratchDir/epicName's epic.yaml sidecar (if any),
// hands it to mutate, and writes it back only when mutate reports a change —
// the shared idempotency check both StampEpicStarted and StampEpicCompleted
// rely on.
func stampEpicTiming(scratchDir, epicName string, mutate func(*epicYAML) bool) error {
	epicPath := filepath.Join(scratchDir, epicName)
	yamlPath := filepath.Join(epicPath, "epic.yaml")

	var wire epicYAML
	if raw, err := os.ReadFile(yamlPath); err == nil {
		if err := yaml.Unmarshal(raw, &wire); err != nil {
			return fmt.Errorf("parsing %s: %w", yamlPath, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if !mutate(&wire) {
		return nil
	}

	out, err := yaml.Marshal(wire)
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", yamlPath, err)
	}
	if err := os.MkdirAll(epicPath, 0755); err != nil {
		return err
	}
	return writeFileAtomic(yamlPath, out)
}

// writeFileAtomic replaces path's content via a same-directory temp file
// plus rename, so a concurrent reader never observes a torn/truncated write.
// Duplicated from ralphloop's/schema's writeFileAtomic (a ~15-line helper)
// rather than exported cross-package, per
// .scratch/ralph-tickets-visibility/issues/02-tickets-set-cli.md's Answer.
// Shared within this package by both epic.yaml sidecar writes and
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
