package git

import (
	"strings"
	"time"
)

type CommitDetails struct {
	FullHash    string
	Hash        string
	AuthorName  string
	AuthorShort string
	Subject     string
	Body        string
	Date        time.Time
	Decorations []RefDecoration
}

func CommitDetailsForRef(repoRoot, ref string) (CommitDetails, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}
	out, _, err := run(repoRoot, []string{
		"show",
		"--no-patch",
		"--date=iso-strict",
		"--format=format:%x1f%H%x1f%h%x1f%an%x1f%aI%x1f%s%x1f%D%x1f%B",
		ref,
	})
	if err != nil {
		return CommitDetails{}, err
	}
	parts := strings.SplitN(out, "\x1f", 8)
	if len(parts) < 8 {
		return CommitDetails{}, nil
	}
	date, _ := time.Parse(time.RFC3339, strings.TrimSpace(parts[4]))
	return CommitDetails{
		FullHash:    strings.TrimSpace(parts[1]),
		Hash:        strings.TrimSpace(parts[2]),
		AuthorName:  strings.TrimSpace(parts[3]),
		AuthorShort: initials(parts[3]),
		Date:        date,
		Subject:     strings.TrimSpace(parts[5]),
		Decorations: parseDecorations(parts[6]),
		Body:        strings.TrimRight(parts[7], "\n"),
	}, nil
}
