package git

import (
	"fmt"
	"strings"
)

type CommitInfo struct {
	Hash    string
	Message string
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
	out, _, err := run(repoRoot, []string{"log", "-1", "--pretty=format:%h\t%s", ref})
	if err != nil {
		return CommitInfo{}, err
	}
	parts := strings.SplitN(out, "\t", 2)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return CommitInfo{}, fmt.Errorf("unable to parse commit info for %s", ref)
	}
	msg := ""
	if len(parts) > 1 {
		msg = strings.TrimSpace(parts[1])
	}
	return CommitInfo{Hash: strings.TrimSpace(parts[0]), Message: msg}, nil
}
