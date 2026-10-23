package tickets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/elentok/gx/fsutil"
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
		epic.Tickets = append(epic.Tickets, loadTicketCached(ticketPath, number, identifier, title, epic.IsMap))
	}

	epic.quarantineInvalidParents()
	epic.flagDanglingBlockers()

	return epic
}

type ticketCacheEntry struct {
	modTime  time.Time
	size     int64
	cachedAt time.Time
	ticket   Ticket
}

var (
	ticketCacheMu sync.Mutex
	ticketCache   = map[string]ticketCacheEntry{}
	// readTicketFile is a seam so tests can count real reads.
	readTicketFile = os.ReadFile
)

// ticketCacheRacy is how recent a file's mtime may be, relative to when it
// was cached, before the entry is not trusted: a same-size edit inside the
// filesystem's mtime granularity would otherwise look unchanged.
const ticketCacheRacy = 2 * time.Second

// loadTicketCached returns the parsed ticket at path, skipping the read and
// parse when its mtime and size match the last load. The file stays the truth:
// any stat difference, or a file modified too close to the cache time, reloads.
func loadTicketCached(path string, number int, identifier, title string, isMap bool) Ticket {
	info, statErr := os.Stat(path)
	if statErr != nil {
		return loadTicketFile(path, number, identifier, title, isMap)
	}

	ticketCacheMu.Lock()
	entry, ok := ticketCache[path]
	ticketCacheMu.Unlock()
	if ok && entry.size == info.Size() && entry.modTime.Equal(info.ModTime()) &&
		entry.cachedAt.Sub(entry.modTime) > ticketCacheRacy {
		return cloneTicket(entry.ticket)
	}

	ticket := loadTicketFile(path, number, identifier, title, isMap)
	ticketCacheMu.Lock()
	ticketCache[path] = ticketCacheEntry{
		modTime:  info.ModTime(),
		size:     info.Size(),
		cachedAt: time.Now(),
		ticket:   cloneTicket(ticket),
	}
	ticketCacheMu.Unlock()
	return ticket
}

func cloneTicket(t Ticket) Ticket {
	t.BlockedBy = slices.Clone(t.BlockedBy)
	t.Mutes = slices.Clone(t.Mutes)
	if t.Parent != nil {
		p := *t.Parent
		t.Parent = &p
	}
	return t
}

func loadTicketFile(ticketPath string, number int, identifier, title string, isMap bool) Ticket {
	ticket := Ticket{
		Number:     number,
		Identifier: identifier,
		Title:      title,
		Path:       ticketPath,
	}

	raw, err := readTicketFile(ticketPath)
	if err != nil {
		ticket.ReadErr = err.Error()
		return ticket
	}

	parse := schema.ParseTicketFromRaw
	if isMap {
		parse = schema.ParseTicketFromRawUnshimmed
	}
	parsed, err := parse(string(raw), ticketPath)
	if err != nil {
		ticket.ReadErr = err.Error()
		return ticket
	}

	ticket.Type = string(parsed.Type)
	ticket.BlockedBy = idsToStrings(parsed.BlockedBy)
	ticket.Parent = idToStringPtr(parsed.Parent)
	ticket.Status = string(parsed.Status)
	ticket.Body = schema.ParseBody(string(raw))
	if heading := headingTitle(ticket.Body, identifier); heading != "" {
		ticket.Title = heading
	}
	ticket.ActualContextWindow = parsed.ActualContextWindow
	ticket.ExpectedContextWindow = parsed.ExpectedContextWindow
	ticket.ElapsedTime = parsed.ElapsedTime
	ticket.ActualCost = parsed.ActualCost
	ticket.Compactions = parsed.Compactions
	ticket.Commitless = parsed.IsCommitless()
	ticket.Category = string(parsed.Category)
	ticket.ExportedAt = parsed.ExportedAt
	ticket.CompletedAt = parsed.CompletedAt
	ticket.CreatedAt = parsed.CreatedAt
	ticket.OriginalAddress = parsed.OriginalAddress
	ticket.CancelledAt = parsed.CancelledAt
	ticket.TotalTokens = parsed.TotalTokens
	ticket.HadCodex = parsed.HadCodex
	ticket.Friction = parsed.Friction
	ticket.ClaimedAt = parsed.ClaimedAt
	ticket.DoneAt = parsed.DoneAt
	ticket.Notify = parsed.Notify
	ticket.Manual = parsed.Manual
	ticket.NoRecover = parsed.NoRecover
	ticket.Unique = parsed.Unique
	ticket.Agent = parsed.Agent
	ticket.PauseReason = parsed.PauseReason
	ticket.Mutes = parsed.Mutes
	ticket.Base = parsed.Base
	ticket.ResolvedBase = parsed.ResolvedBase
	return ticket
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

// headingTitle returns body's first `# ` heading, without a leading
// "<identifier> — " prefix. It returns "" when there is no heading or the
// heading is only the identifier, so the caller keeps the slug title — the
// slug is lowercase and loses acronyms ("Os notifier"), the heading doesn't.
// Lines inside code fences are skipped: a shell comment is not a heading.
func headingTitle(body, identifier string) string {
	inFence := false
	for line := range strings.Lines(body) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(line, "# ") {
			continue
		}
		heading := strings.TrimSpace(line[2:])
		if rest, ok := cutIDPrefix(heading, identifier); ok {
			heading = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(rest), "—–-:"))
		}
		return heading
	}
	return ""
}

// cutIDPrefix cuts identifier off the start of heading, ignoring leading
// zeros on either side ("1 — X" on ticket 01). It only cuts at a word
// boundary, so "07b split" on ticket 07 is not an id prefix.
func cutIDPrefix(heading, identifier string) (rest string, ok bool) {
	core := strings.TrimLeft(identifier, "0")
	if core == "" {
		return "", false
	}
	rest, ok = strings.CutPrefix(strings.TrimLeft(heading, "0"), core)
	if !ok || rest == "" {
		return rest, ok
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return rest, unicode.IsSpace(r) || strings.ContainsRune("—–-:", r)
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
	CreatedAt   *time.Time `yaml:"created_at"`
	MergedAt    *time.Time `yaml:"merged_at"`
	QueuedAt    *time.Time `yaml:"queued_at"`

	MergeBlockKind   string `yaml:"land_block_kind"`
	MergeBlockReason string `yaml:"land_block_reason"`
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
	epic.Body = schema.ParseBody(string(raw))

	fm, ok := schema.FrontmatterYAML(string(raw))
	if !ok {
		return
	}
	var wire ticketMDYAML
	if err := yaml.Unmarshal([]byte(fm), &wire); err != nil {
		return
	}
	epic.IsMap = wire.Kind == KindMap
	epic.Status = wire.Status
	epic.BlockedBy = wire.BlockedBy
	epic.Base = wire.Base
	if wire.StartedAt != nil {
		epic.StartedAt = *wire.StartedAt
	}
	if wire.CompletedAt != nil {
		epic.CompletedAt = *wire.CompletedAt
	}
	if wire.CreatedAt != nil {
		epic.CreatedAt = *wire.CreatedAt
	}
	if wire.MergedAt != nil {
		epic.MergedAt = *wire.MergedAt
	}
	if wire.QueuedAt != nil {
		epic.QueuedAt = *wire.QueuedAt
	}
	epic.MergeBlockKind = wire.MergeBlockKind
	epic.MergeBlockReason = wire.MergeBlockReason
	loadEpicEvolveFields(epic, fm)
}

// loadEpicEvolveFields reads the evolve-related ticket.md keys one at a time
// rather than through ticketMDYAML, so a malformed value is reported under its
// own field name and doesn't hide the rest of the frontmatter.
func loadEpicEvolveFields(epic *Epic, fm string) {
	var nodes map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &nodes); err != nil {
		return
	}
	decode := func(key string, dst any) {
		node, ok := nodes[key]
		if !ok {
			return
		}
		if err := node.Decode(dst); err != nil {
			epic.FrontmatterErrs = append(epic.FrontmatterErrs, fmt.Sprintf("%s: %v", key, err))
		}
	}
	decode("evolved_at", &epic.EvolvedAt)
	decode("evolve_skipped", &epic.EvolveSkipped)
	decode("evolve_ticket", &epic.EvolveTicket)
	decode("destination_unconfirmed", &epic.DestinationUnconfirmed)
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
	return StampEpicFields(filepath.Join(scratchDir, epicName), map[string]any{"completed_at": now})
}

// StampEpicMerged writes merged_at into epicPath's ticket.md frontmatter,
// once: a merged_at already on disk is left alone.
func StampEpicMerged(epicPath string, now time.Time) error {
	return StampEpicFields(epicPath, map[string]any{"merged_at": now})
}

// StampEpicFields sets each key in fields on the epic's ticket.md frontmatter
// unless the key is already there: an existing stamp is never overwritten, and
// every other key and the body are kept. Retro stamping and `gx evolve skip`
// reuse it for their own keys.
func StampEpicFields(epicPath string, fields map[string]any) error {
	return stampEpicFields(epicPath, fields, false)
}

// OverwriteEpicFields is StampEpicFields for keys a later run legitimately
// replaces (a forced re-run's evolve_ticket): an existing value is rewritten.
func OverwriteEpicFields(epicPath string, fields map[string]any) error {
	return stampEpicFields(epicPath, fields, true)
}

// ClearEpicFields removes each key from the epic's ticket.md frontmatter. A key
// that is not there is skipped, and the file is untouched when none is.
func ClearEpicFields(epicPath string, keys ...string) error {
	return editEpicFrontmatter(epicPath, func(mapping *yaml.Node) (bool, error) {
		changed := false
		for _, key := range keys {
			if i := mappingKeyIndex(mapping, key); i >= 0 {
				mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
				changed = true
			}
		}
		return changed, nil
	})
}

func stampEpicFields(epicPath string, fields map[string]any, overwrite bool) error {
	return editEpicFrontmatter(epicPath, func(mapping *yaml.Node) (bool, error) {
		changed := false
		for key, value := range fields {
			idx := mappingKeyIndex(mapping, key)
			if idx >= 0 && !overwrite {
				continue
			}
			var val yaml.Node
			if err := val.Encode(value); err != nil {
				return false, fmt.Errorf("encoding %s: %w", key, err)
			}
			if idx >= 0 {
				mapping.Content[idx+1] = &val
			} else {
				mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &val)
			}
			changed = true
		}
		return changed, nil
	})
}

// editEpicFrontmatter runs edit on the epic's ticket.md frontmatter mapping
// under the ticket lock, and writes the file back only when edit reports a change.
func editEpicFrontmatter(epicPath string, edit func(mapping *yaml.Node) (bool, error)) error {
	ticketPath := filepath.Join(epicPath, "ticket.md")
	if _, err := os.Stat(ticketPath); err != nil {
		return fmt.Errorf("epic %s has no ticket.md: %w", epicPath, err)
	}
	unlock, err := schema.LockTicket(ticketPath)
	if err != nil {
		return err
	}
	defer unlock()
	raw, err := os.ReadFile(ticketPath)
	if err != nil {
		return err
	}
	fm, body, ok := schema.SplitFrontmatter(string(raw))
	if !ok {
		return fmt.Errorf("%s: no frontmatter to stamp into", ticketPath)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", ticketPath, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s: frontmatter is not a mapping", ticketPath)
	}
	mapping := doc.Content[0]
	changed, err := edit(mapping)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	out, err := yaml.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", ticketPath, err)
	}
	return fsutil.WriteFileAtomic(ticketPath, []byte("---\n"+string(out)+"---\n"+body))
}

// mappingKeyIndex returns the index of key in mapping.Content, or -1 when absent.
// The value node sits at the returned index + 1.
func mappingKeyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
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
	return fsutil.WriteFileAtomic(path, []byte("---\n"+string(out)+"---\n"+body))
}

// setMappingTime sets key to t in mapping, appending the key when absent.
func setMappingTime(mapping *yaml.Node, key string, t time.Time) {
	var val yaml.Node
	_ = val.Encode(t)
	if i := mappingKeyIndex(mapping, key); i >= 0 {
		mapping.Content[i+1] = &val
		return
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &val)
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
