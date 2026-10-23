package git

import (
	"fmt"
	"strings"
	"time"
)

// FileOrigin is where a file first appeared in history.
type FileOrigin struct {
	// Path is the file's path in its first commit, relative to the repo root.
	Path string
	// At is the first commit's commit time.
	At time.Time
	// Root is true when the first commit is a root commit (it has no parent).
	Root bool
}

// FirstCommitOfFile returns the first commit that contains path (relative to
// repoRoot) and the path it had there, following renames. ok is false when the
// file has no history, e.g. it is untracked.
func FirstCommitOfFile(repoRoot, path string) (origin FileOrigin, ok bool, err error) {
	out, _, err := run(repoRoot, []string{
		"log", "--follow", "--name-status", "--format=@%cI\t%P", "--", path,
	})
	if err != nil {
		return FileOrigin{}, false, err
	}
	// Newest commit first; the last record is the oldest.
	var header, status string
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "@"):
			header, status = line[1:], ""
		case line != "":
			status = line
		}
	}
	if header == "" || status == "" {
		return FileOrigin{}, false, nil
	}
	when, parents, _ := strings.Cut(header, "\t")
	at, err := time.Parse(time.RFC3339, when)
	if err != nil {
		return FileOrigin{}, false, fmt.Errorf("parsing commit time %q: %w", when, err)
	}
	fields := strings.Split(status, "\t")
	return FileOrigin{Path: fields[len(fields)-1], At: at, Root: parents == ""}, true, nil
}
