package check

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/tickets"
)

// Retention is how long run records and run logs are kept.
const Retention = 30 * 24 * time.Hour

// How a record's branch relates to the work: a ralph-loop ticket branch, the
// project's trunk, or anything else.
const (
	LinkTicket = "ticket"
	LinkTrunk  = "trunk"
	LinkOther  = "other"
)

// KindRecord is one kind's line of a Record.
type KindRecord struct {
	Kind       string `json:"kind"`
	Status     Status `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	// Failed names the failing tests of the brief runs this kind started. A
	// configured command that never calls `gx brief` has none.
	Failed []string `json:"failed,omitempty"`
}

// Record is one line of <data>/check/<project>.jsonl: what one `gx check` run
// did and the tree it checked.
type Record struct {
	Run        string       `json:"run"`
	Time       string       `json:"time"`
	Kinds      []KindRecord `json:"kinds"`
	Exit       int          `json:"exit"`
	Scope      string       `json:"scope"`
	KindFilter string       `json:"kind_filter"`
	Branch     string       `json:"branch"`
	Worktree   string       `json:"worktree"`
	Head       string       `json:"head"`
	TreeID     string       `json:"tree_id"`
	TreeError  string       `json:"tree_error,omitempty"` // set when TreeID is empty
	Link       string       `json:"link"`
	Epic       string       `json:"epic,omitempty"`   // set for LinkTicket
	Ticket     string       `json:"ticket,omitempty"` // set for LinkTicket
}

func (m Mode) scope() string {
	return [...]string{"changed", "staged", "since", "full"}[m]
}

func recordPath(dataDir, project string) string {
	return filepath.Join(dataDir, "check", project+".jsonl")
}

// ProjectIn is the project name records of the repo at dir are filed under:
// the registered project's name in the ticket store at storePath, else the
// repo root's base name.
func ProjectIn(storePath, dir string) string {
	return projectFor(func(root string) (string, error) { return tickets.RootIn(storePath, root) }, dir)
}

// ProjectForDirs is ProjectIn with the store path read from dirs.
func ProjectForDirs(dirs config.Dirs, dir string) string {
	return projectFor(func(root string) (string, error) { return tickets.RootForDirs(dirs, root) }, dir)
}

func projectFor(rootFor func(string) (string, error), dir string) string {
	root, err := repoRoot(dir)
	if err != nil {
		return filepath.Base(dir)
	}
	if projectDir, err := rootFor(root); err == nil {
		return tickets.ProjectName(projectDir)
	}
	return filepath.Base(root)
}

// ReadRecords returns the records of project, oldest first. A missing file is
// empty; a line that does not decode is skipped.
func ReadRecords(dataDir, project string) ([]Record, error) {
	data, err := os.ReadFile(recordPath(dataDir, project))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []Record
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		var r Record
		if len(line) > 0 && json.Unmarshal(line, &r) == nil {
			recs = append(recs, r)
		}
	}
	return recs, nil
}

// FilterRecords keeps the records of branch and tree; an empty one matches any.
func FilterRecords(recs []Record, branch, tree string) []Record {
	return slices.DeleteFunc(slices.Clone(recs), func(r Record) bool {
		return (branch != "" && r.Branch != branch) || (tree != "" && r.TreeID != tree)
	})
}

// newRecord describes the finished run.
func (r *runner) newRecord(opts Options, started time.Time, res Result) Record {
	rec := Record{
		Run:        r.runID,
		Time:       started.UTC().Format(time.RFC3339),
		Exit:       res.Exit,
		Scope:      r.mode.scope(),
		KindFilter: strings.Join(opts.Kinds, ","),
		Worktree:   r.root,
		TreeID:     r.treeID,
		TreeError:  r.treeErr,
		Link:       LinkOther,
	}
	rec.Branch, _ = git.CurrentBranch(r.root)
	rec.Head, _ = git.RevParse(r.root, "HEAD")
	if epic, id, ok := tickets.ParseIterationBranch(rec.Branch); ok {
		rec.Link, rec.Epic, rec.Ticket = LinkTicket, epic, id
	} else if trunk, err := tickets.TrunkIn(r.ticketStore, r.root); err == nil && trunk == rec.Branch {
		rec.Link = LinkTrunk
	}

	failed := map[string][]string{}
	if lines, err := brief.ReadIndex(r.dataDir); err == nil {
		for _, l := range lines {
			if l.CheckRun == r.runID {
				failed[l.CheckKind] = append(failed[l.CheckKind], l.Failed...)
			}
		}
	}
	for _, k := range res.Kinds {
		rec.Kinds = append(rec.Kinds, KindRecord{
			Kind: k.Kind, Status: k.Status, DurationMS: k.Duration.Milliseconds(), Failed: failed[k.Kind],
		})
	}
	return rec
}

// appendRecord sweeps what is older than Retention and appends rec in one
// O_APPEND write, both under the project's lock. A failed sweep never fails
// the run.
func appendRecord(dataDir, project string, rec Record, now time.Time) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	path := recordPath(dataDir, project)
	unlock, err := brief.LockFile(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	cutoff := now.Add(-Retention)
	sweepRecords(path, cutoff)
	sweepRunDirs(filepath.Dir(path), cutoff)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

func sweepRecords(path string, cutoff time.Time) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var kept bytes.Buffer
	dropped := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var r Record
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			if t, err := time.Parse(time.RFC3339, r.Time); err == nil && t.Before(cutoff) {
				dropped = true
				continue
			}
		}
		kept.Write(sc.Bytes())
		kept.WriteByte('\n')
	}
	if !dropped {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, kept.Bytes(), 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

func sweepRunDirs(checkDir string, cutoff time.Time) {
	entries, err := os.ReadDir(checkDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(checkDir, e.Name()))
		}
	}
}
