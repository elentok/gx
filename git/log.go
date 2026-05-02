package git

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Commit is a single git commit with abbreviated hash and subject line.
type Commit struct {
	FullHash string
	Hash     string
	Subject  string
	Date     time.Time
}

type BranchHistoryClass string

const (
	BranchHistoryShared     BranchHistoryClass = "shared"
	BranchHistoryLocalOnly  BranchHistoryClass = "local_only"
	BranchHistoryRemoteOnly BranchHistoryClass = "remote_only"
)

type BranchHistoryCommit struct {
	Commit
	Class BranchHistoryClass
}

type RefDecorationKind string

const (
	RefDecorationLocalBranch  RefDecorationKind = "local_branch"
	RefDecorationRemoteBranch RefDecorationKind = "remote_branch"
	RefDecorationTag          RefDecorationKind = "tag"
)

type RefDecoration struct {
	Name string
	Kind RefDecorationKind
}

type LogEntry struct {
	FullHash    string
	Hash        string
	AuthorName  string
	AuthorShort string
	Subject     string
	Date        time.Time
	Graph       string
	Decorations []RefDecoration
}

// CommitsSinceMain returns commits on branch that are not reachable from the
// repo's local main branch, ordered newest first.
func CommitsSinceMain(repo Repo, branch string) ([]Commit, error) {
	return commitsBetween(repo, repo.MainBranch, branch)
}

// CommitsBehindMain returns commits on main that are not reachable from branch,
// ordered newest first.
func CommitsBehindMain(repo Repo, branch string) ([]Commit, error) {
	return commitsBetween(repo, branch, repo.MainBranch)
}

// HeadCommit returns the latest commit on the given branch.
func HeadCommit(repoRoot, branch string) (Commit, error) {
	out, _, err := run(repoRoot, []string{"log", "-1", "--pretty=format:%H\t%h\t%ci\t%s", branch})
	if err != nil || out == "" {
		return Commit{}, err
	}
	fullHash, rest, _ := strings.Cut(out, "\t")
	hash, rest, _ := strings.Cut(rest, "\t")
	dateStr, subject, _ := strings.Cut(rest, "\t")
	date, _ := time.Parse("2006-01-02 15:04:05 -0700", dateStr)
	return Commit{FullHash: fullHash, Hash: hash, Subject: subject, Date: date}, nil
}

func ResolveCommitish(repoRoot, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}
	out, _, err := run(repoRoot, []string{"rev-parse", "--verify", ref + "^{commit}"})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func LogEntries(repoRoot, startRef string, limit int) ([]LogEntry, error) {
	if strings.TrimSpace(startRef) == "" {
		startRef = "HEAD"
	}
	args := []string{
		"log",
		"--graph",
		"--decorate=short",
		"--date=iso-strict",
		"--format=format:%x1f%H%x1f%h%x1f%an%x1f%aI%x1f%s%x1f%D",
	}
	if limit > 0 {
		args = append(args, "--max-count", strconv.Itoa(limit))
	}
	args = append(args, startRef)

	out, _, err := run(repoRoot, args)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	entries := make([]LogEntry, 0, strings.Count(out, "\n")+1)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		graph, rest, ok := strings.Cut(line, "\x1f")
		if !ok {
			continue
		}
		fields := strings.Split(rest, "\x1f")
		if len(fields) < 6 {
			continue
		}
		date, _ := time.Parse(time.RFC3339, strings.TrimSpace(fields[3]))
		entries = append(entries, LogEntry{
			FullHash:    strings.TrimSpace(fields[0]),
			Hash:        strings.TrimSpace(fields[1]),
			AuthorName:  strings.TrimSpace(fields[2]),
			AuthorShort: initials(fields[2]),
			Date:        date,
			Subject:     strings.TrimSpace(fields[4]),
			Graph:       strings.TrimRight(graph, " "),
			Decorations: parseDecorations(fields[5]),
		})
	}
	return entries, nil
}

// CommitsBetween returns commits reachable from toRef but not fromRef, ordered
// newest first.
func CommitsBetween(repo Repo, fromRef, toRef string) ([]Commit, error) {
	return commitsBetween(repo, fromRef, toRef)
}

func commitsBetween(repo Repo, fromRef, toRef string) ([]Commit, error) {
	mergeBase, _, err := run(repo.Root, []string{"merge-base", fromRef, toRef})
	if err != nil {
		// No merge base (e.g. orphan branch) - return empty rather than error
		return nil, nil
	}

	return commitsFromRange(repo.Root, mergeBase, toRef)
}

// BranchHistorySinceMain returns branch history since the repo's remote
// mainline ref (for example origin/main or origin/master) when available.
func BranchHistorySinceMain(repo Repo, branch, upstreamRef string) ([]BranchHistoryCommit, error) {
	baseRef := DefaultMainRemoteRef(repo.Root)
	if strings.TrimSpace(baseRef) == "" {
		return nil, nil
	}

	mergeBase, _, err := run(repo.Root, []string{"merge-base", baseRef, branch})
	if err != nil {
		return nil, nil
	}

	localCommits, err := commitsFromRange(repo.Root, strings.TrimSpace(mergeBase), branch)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(upstreamRef) == "" {
		history := make([]BranchHistoryCommit, 0, len(localCommits))
		for _, commit := range localCommits {
			history = append(history, BranchHistoryCommit{
				Commit: commit,
				Class:  BranchHistoryLocalOnly,
			})
		}
		return history, nil
	}

	upstreamCommits, err := commitsFromRange(repo.Root, strings.TrimSpace(mergeBase), upstreamRef)
	if err != nil {
		return nil, err
	}

	localByHash := make(map[string]Commit, len(localCommits))
	upstreamByHash := make(map[string]Commit, len(upstreamCommits))
	for _, commit := range localCommits {
		localByHash[commit.FullHash] = commit
	}
	for _, commit := range upstreamCommits {
		upstreamByHash[commit.FullHash] = commit
	}

	history := make([]BranchHistoryCommit, 0, len(localCommits)+len(upstreamCommits))
	seen := make(map[string]bool, len(localCommits)+len(upstreamCommits))
	for _, commit := range localCommits {
		class := BranchHistoryLocalOnly
		if _, ok := upstreamByHash[commit.FullHash]; ok {
			class = BranchHistoryShared
		}
		history = append(history, BranchHistoryCommit{Commit: commit, Class: class})
		seen[commit.FullHash] = true
	}
	for _, commit := range upstreamCommits {
		if seen[commit.FullHash] {
			continue
		}
		class := BranchHistoryRemoteOnly
		if _, ok := localByHash[commit.FullHash]; ok {
			class = BranchHistoryShared
		}
		history = append(history, BranchHistoryCommit{Commit: commit, Class: class})
	}

	sort.Slice(history, func(i, j int) bool {
		return history[i].Date.After(history[j].Date)
	})

	return history, nil
}

func commitsFromRange(repoRoot, fromExclusive, toRef string) ([]Commit, error) {
	out, _, err := run(repoRoot, []string{"log", "--pretty=format:%H\t%h\t%ci\t%s", strings.TrimSpace(fromExclusive) + ".." + toRef})
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		fullHash, rest, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		hash, rest, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		dateStr, subject, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		date, _ := time.Parse("2006-01-02 15:04:05 -0700", dateStr)
		commits = append(commits, Commit{
			FullHash: fullHash,
			Hash:     hash,
			Subject:  subject,
			Date:     date,
		})
	}
	return commits, nil
}
