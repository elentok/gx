package git

import (
	"fmt"
	"strings"
	"time"
)

type CommitInfo struct {
	Hash    string
	Message string
	Date    time.Time
}

type PushDivergence struct {
	Branch     string
	Remote     string
	Upstream   string
	Local      CommitInfo
	RemoteHead CommitInfo
}

// DetectPushDivergence fetches origin when an upstream branch exists and reports
// divergence between local branch and its upstream.
func DetectPushDivergence(worktreePath, branch string) (*PushDivergence, error) {
	upstream := UpstreamBranch(worktreePath, branch)
	if upstream == "" {
		return nil, nil
	}
	remote := BranchRemote(Repo{Root: worktreePath}, branch)
	if err := Fetch(worktreePath, "origin"); err != nil {
		return nil, err
	}
	status, err := syncBetween(worktreePath, branch, upstream)
	if err != nil {
		return nil, err
	}
	if status.Name != StatusDiverged {
		return nil, nil
	}
	local, err := commitInfo(worktreePath, branch)
	if err != nil {
		return nil, err
	}
	remoteHead, err := commitInfo(worktreePath, upstream)
	if err != nil {
		return nil, err
	}
	return &PushDivergence{
		Branch:     branch,
		Remote:     remote,
		Upstream:   upstream,
		Local:      local,
		RemoteHead: remoteHead,
	}, nil
}

func commitInfo(repoRoot, ref string) (CommitInfo, error) {
	out, _, err := run(repoRoot, []string{"log", "-1", "--pretty=format:%h\t%ci\t%s", ref})
	if err != nil {
		return CommitInfo{}, err
	}
	parts := strings.SplitN(out, "\t", 3)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return CommitInfo{}, fmt.Errorf("unable to parse commit info for %s", ref)
	}
	var date time.Time
	if len(parts) > 1 {
		date, _ = time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(parts[1]))
	}
	msg := ""
	if len(parts) > 2 {
		msg = strings.TrimSpace(parts[2])
	}
	return CommitInfo{Hash: strings.TrimSpace(parts[0]), Message: msg, Date: date}, nil
}
