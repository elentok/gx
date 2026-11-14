package rangediff

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/git/internal/run"
)

// MaxCommits caps the commit list; past it CommitList.Truncated is set.
const MaxCommits = 500

// ticketTrailerKey is the trailer ralphloop/merge stamps on every commit.
const ticketTrailerKey = "Ralph-Loop-Ticket"

// EpicName is one epic's current name and its branch name. A rename keeps the
// branch, so an old trailer holds the branch name.
type EpicName struct {
	Name   string
	Branch string
}

// Commit is one commit of the range.
type Commit struct {
	SHA     string
	Subject string
	Time    time.Time
	// Ticket is the canonical "<project>:<epic>/<id>", or "" (null) when the
	// commit has no usable trailer.
	Ticket string
	Merge  bool
	// Files are the paths the commit changed; empty for a merge or an empty commit.
	Files []string
}

// CommitList is the first MaxCommits commits of a range, oldest first.
type CommitList struct {
	Commits   []Commit
	Truncated bool
}

// UnknownCommitError is returned when a commit filter names a SHA the commit
// list did not return.
type UnknownCommitError struct{ SHA string }

func (e *UnknownCommitError) Error() string {
	return fmt.Sprintf("commit %q is not in the returned commit list", e.SHA)
}

const (
	recSep   = "\x1e"
	fieldSep = "\x1f"
)

// ListCommits lists the range's commits in the order of the work. project and
// epics turn each commit's ticket trailer into a canonical address.
func ListCommits(dir string, r Range, project string, epics []EpicName) (CommitList, error) {
	if r.NoCommonHistory {
		return CommitList{}, nil
	}
	shas, truncated, err := rangeSHAs(dir, r)
	if err != nil {
		return CommitList{}, err
	}
	if len(shas) == 0 {
		return CommitList{Commits: []Commit{}}, nil
	}
	format := strings.Join([]string{recSep + "%H", "%P", "%at", "%s",
		"%B"}, fieldSep)
	args := append([]string{"-c", "core.quotepath=off", "log", "--no-walk=unsorted", "-z", "--name-only",
		"--no-color", "--format=" + format}, shas...)
	out, _, err := run.RunRaw(dir, run.NonInteractiveEnv(), args...)
	if err != nil {
		return CommitList{}, err
	}
	list := CommitList{Commits: []Commit{}, Truncated: truncated}
	for _, rec := range strings.Split(string(out), recSep)[1:] {
		c, err := parseCommit(rec, project, epics)
		if err != nil {
			return CommitList{}, err
		}
		list.Commits = append(list.Commits, c)
	}
	return list, nil
}

// rangeSHAs returns up to MaxCommits SHAs oldest first and whether more exist.
func rangeSHAs(dir string, r Range) ([]string, bool, error) {
	out, _, err := run.Exec(dir, []string{"rev-list", "--reverse", "--topo-order", r.MergeBase + ".." + r.SecondSHA})
	if err != nil {
		return nil, false, err
	}
	if out == "" {
		return nil, false, nil
	}
	shas := strings.Split(out, "\n")
	if len(shas) > MaxCommits {
		return shas[:MaxCommits], true, nil
	}
	return shas, false, nil
}

func parseCommit(rec, project string, epics []EpicName) (Commit, error) {
	head, rest, _ := strings.Cut(rec, "\x00")
	f := strings.Split(head, fieldSep)
	if len(f) != 5 {
		return Commit{}, fmt.Errorf("rangediff: bad commit record %q", head)
	}
	at, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil {
		return Commit{}, fmt.Errorf("rangediff: bad author time %q", f[2])
	}
	c := Commit{
		SHA:     f[0],
		Subject: f[3],
		Time:    time.Unix(at, 0).UTC(),
		Merge:   len(strings.Fields(f[1])) > 1,
		Ticket:  canonicalTicket(f[4], project, epics),
	}
	for p := range strings.SplitSeq(rest, "\x00") {
		if p = strings.TrimPrefix(p, "\n"); p != "" {
			c.Files = append(c.Files, p)
		}
	}
	return c, nil
}

// canonicalTicket maps the commit message's ticket trailer to one address. It
// scans every line of the message rather than git's trailer block, so a
// squash body with several stamped paragraphs counts all of them. More than
// one line or an unknown epic gives "".
func canonicalTicket(message, project string, epics []EpicName) string {
	var value string
	n := 0
	for line := range strings.SplitSeq(message, "\n") {
		if v, ok := strings.CutPrefix(line, ticketTrailerKey+":"); ok {
			value = v
			n++
		}
	}
	if n != 1 {
		return ""
	}
	v := strings.TrimSpace(value)
	if i := strings.IndexByte(v, ':'); i >= 0 {
		v = v[i+1:]
	}
	i := strings.LastIndexByte(v, '/')
	if i <= 0 || i == len(v)-1 {
		return ""
	}
	epic, id := v[:i], v[i+1:]
	for _, e := range epics {
		if epic == e.Name || epic == e.Branch {
			return project + ":" + e.Name + "/" + id
		}
	}
	return ""
}

// selectedCommits checks every SHA against the commit call's list and returns
// the selected commits in range order.
func selectedCommits(dir string, r Range, shas []string) ([]Commit, error) {
	if r.NoCommonHistory {
		if len(shas) > 0 {
			return nil, &UnknownCommitError{SHA: shas[0]}
		}
		return nil, nil
	}
	all, _, err := rangeSHAs(dir, r)
	if err != nil {
		return nil, err
	}
	inRange := map[string]bool{}
	for _, s := range all {
		inRange[s] = true
	}
	want := map[string]bool{}
	for _, s := range shas {
		if !inRange[s] {
			return nil, &UnknownCommitError{SHA: s}
		}
		want[s] = true
	}
	var picked []Commit
	for _, s := range all {
		if !want[s] {
			continue
		}
		c, err := commitInfo(dir, s)
		if err != nil {
			return nil, err
		}
		picked = append(picked, c)
	}
	return picked, nil
}

func commitInfo(dir, sha string) (Commit, error) {
	out, _, err := run.Exec(dir, []string{"log", "-1", "--format=%P" + fieldSep + "%s", sha})
	if err != nil {
		return Commit{}, err
	}
	parents, subject, _ := strings.Cut(out, fieldSep)
	return Commit{SHA: sha, Subject: subject, Merge: len(strings.Fields(parents)) > 1}, nil
}

// ListFilesIn lists the files the selected commits changed. A file several
// commits changed appears once, with their counts added and the net status.
func ListFilesIn(dir string, r Range, shas []string) (FileList, error) {
	picked, err := selectedCommits(dir, r, shas)
	if err != nil {
		return FileList{}, err
	}
	files := []File{}
	index := map[string]int{}
	for _, c := range picked {
		if c.Merge {
			continue
		}
		cf, err := diffFiles(dir, []string{"show", "--format="}, []string{c.SHA, "--"})
		if err != nil {
			return FileList{}, err
		}
		for _, f := range cf {
			// A rename continues the entry of its source path.
			key := f.Path
			if f.OldPath != "" {
				if _, ok := index[f.OldPath]; ok {
					key = f.OldPath
				}
			}
			i, seen := index[key]
			if !seen {
				index[f.Path] = len(files)
				files = append(files, f)
				continue
			}
			files[i] = foldCommitFile(files[i], f)
			if key != f.Path {
				delete(index, key)
				index[f.Path] = i
			}
		}
	}
	return capFiles(files), nil
}

// foldCommitFile combines a file's entry so far with its change in a later
// selected commit. Added or renamed stays so across later edits.
func foldCommitFile(prev, next File) File {
	next.Added += prev.Added
	next.Deleted += prev.Deleted
	next.Large = next.Large || prev.Large
	next.Binary = next.Binary || prev.Binary
	if (prev.Status == "A" || prev.Status == "R") && next.Status == "M" {
		next.Status = prev.Status
		next.OldPath = prev.OldPath
	} else if prev.Status == "R" && next.Status == "R" {
		next.OldPath = prev.OldPath
	}
	return next
}

// CommitPatch is one selected commit's patch of a file.
type CommitPatch struct {
	SHA     string
	Subject string
	Patch   string
}

// PatchesIn returns the file's patch per selected commit in range order.
// Commits that did not touch the file and merge commits give no entry.
func PatchesIn(dir string, r Range, shas []string, f File) ([]CommitPatch, error) {
	picked, err := selectedCommits(dir, r, shas)
	if err != nil {
		return nil, err
	}
	var out []CommitPatch
	if f.Binary {
		return out, nil
	}
	for _, c := range picked {
		if c.Merge {
			continue
		}
		args := []string{"--literal-pathspecs", "show", "--format=", "--no-color", "--find-renames",
			"--no-ext-diff", c.SHA, "--"}
		if f.OldPath != "" {
			args = append(args, f.OldPath)
		}
		args = append(args, f.Path)
		raw, _, err := run.RunRaw(dir, run.NonInteractiveEnv(), args...)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			continue
		}
		out = append(out, CommitPatch{SHA: c.SHA, Subject: c.Subject, Patch: string(raw)})
	}
	return out, nil
}
