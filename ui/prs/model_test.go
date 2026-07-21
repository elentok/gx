package prs

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/keys"
)

func sendModel(m Model, msg tea.Msg) Model {
	updated, _ := m.Update(msg)
	return updated.(Model)
}

// loadPRs delivers both the open- and closed-PR fetch results as the
// separate messages the concurrent load pipeline actually produces (see
// issues/09-load-time-batched-fetch.md), keeping call sites below as terse as
// the old single-message form.
func loadPRs(m Model, prs []git.PR, anyPRs bool, err error, closedPRs []git.ClosedPR) Model {
	m = sendModel(m, openPRsLoadedMsg{prs: prs, anyPRs: anyPRs, err: err})
	m = sendModel(m, closedPRsLoadedMsg{closedPRs: closedPRs})
	return m
}

// resolveMsgs recursively runs cmd (unwrapping any nested tea.BatchMsg) and
// returns every leaf message it produces, so a test can assert a refresh path
// actually reloads both sections rather than just returning "some cmd".
func resolveMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, sub := range batch {
			msgs = append(msgs, resolveMsgs(sub)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func containsMsgTypes(msgs []tea.Msg) (sawOpen, sawClosed bool) {
	for _, msg := range msgs {
		switch msg.(type) {
		case openPRsLoadedMsg:
			sawOpen = true
		case closedPRsLoadedMsg:
			sawClosed = true
		}
	}
	return sawOpen, sawClosed
}

func TestModelRendersLoadingPlaceholder(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	content := m.View().Content
	if !strings.Contains(content, "loading") {
		t.Fatalf("expected loading placeholder content, got:\n%s", content)
	}
}

func TestModelRendersNoPRsFoundPlaceholder(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, false, nil, nil)

	content := m.View().Content
	if !strings.Contains(content, "no PRs found") {
		t.Fatalf("expected 'no PRs found' placeholder, got:\n%s", content)
	}
}

func TestModelRendersNoOpenPRsPlaceholder(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, true, nil, nil)

	content := m.View().Content
	if !strings.Contains(content, "no open PRs") {
		t.Fatalf("expected 'no open PRs' placeholder, got:\n%s", content)
	}
}

func TestModelRendersPRRows(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		{Number: 12, Title: "Add widget", UpdatedAt: time.Now()},
		{Number: 34, Title: "Draft feature", IsDraft: true, UpdatedAt: time.Now()},
	}, true, nil, nil)

	content := m.View().Content
	if !strings.Contains(content, "#12") || !strings.Contains(content, "Add widget") {
		t.Fatalf("expected PR #12 row, got:\n%s", content)
	}
	if !strings.Contains(content, "#34") || !strings.Contains(content, "DRAFT") {
		t.Fatalf("expected draft PR #34 with DRAFT badge, got:\n%s", content)
	}
}

func TestModelRendersRepoNameInAllReposMode(t *testing.T) {
	m := NewModelWithScope("/repo", ui.Settings{}, keys.Manager{}, true)
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		{Number: 12, Title: "Add widget", UpdatedAt: time.Now(), Repo: "acme/widgets"},
	}, true, nil, nil)

	content := m.View().Content
	if !strings.Contains(content, "widgets") {
		t.Fatalf("expected repo short name 'widgets' before #12, got:\n%s", content)
	}
	if strings.Contains(content, "acme/widgets") {
		t.Fatalf("expected only the short repo name (no owner), got:\n%s", content)
	}
}

func TestModelTruncatesLongRepoName(t *testing.T) {
	m := NewModelWithScope("/repo", ui.Settings{}, keys.Manager{}, true)
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		{Number: 12, Title: "Add widget", UpdatedAt: time.Now(), Repo: "acme/a-very-long-repository-name-indeed"},
	}, true, nil, nil)

	content := m.View().Content
	if !strings.Contains(content, "…") {
		t.Fatalf("expected long repo name to be truncated with an ellipsis, got:\n%s", content)
	}
	if strings.Contains(content, "a-very-long-repository-name-indeed") {
		t.Fatalf("expected long repo name to be truncated, got full name in:\n%s", content)
	}
}

func TestModelOmitsRepoNameInCurrentRepoMode(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		{Number: 12, Title: "Add widget", UpdatedAt: time.Now()},
	}, true, nil, nil)

	content := m.View().Content
	if strings.Contains(content, "widgets") {
		t.Fatalf("expected no repo name in current-repo mode, got:\n%s", content)
	}
}

func TestModelRendersFacetsAndMarker(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		{
			Number:            12,
			Title:             "Add widget",
			UpdatedAt:         time.Now(),
			StatusCheckRollup: []git.PRStatusCheck{{Status: "COMPLETED", Conclusion: "SUCCESS"}},
			ReviewDecision:    "APPROVED",
			Mergeable:         "MERGEABLE",
			Reviews:           []git.PRReview{{Body: "lgtm"}},
		},
	}, true, nil, nil)

	content := m.View().Content
	if !strings.Contains(content, "1c") {
		t.Fatalf("expected comment count facet, got:\n%s", content)
	}
}

func TestModelRendersMergeConflictFacet(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		{Number: 12, Title: "Add widget", UpdatedAt: time.Now(), Mergeable: "CONFLICTING"},
	}, true, nil, nil)

	content := m.View().Content
	if !strings.Contains(content, "⚠") {
		t.Fatalf("expected conflict marker facet, got:\n%s", content)
	}
}

func TestModelRendersLoadError(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, false, errBoom, nil)

	content := m.View().Content
	if !strings.Contains(content, "error") || !strings.Contains(content, "boom") {
		t.Fatalf("expected raw wrapped error content, got:\n%s", content)
	}
}

func TestModelRendersGHNotInstalledError(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, false, &git.PRListError{Kind: git.PRListErrorGHNotInstalled, Err: errBoom}, nil)

	content := m.View().Content
	if !strings.Contains(content, "gh not found") || !strings.Contains(content, "install") {
		t.Fatalf("expected gh-not-installed hint, got:\n%s", content)
	}
}

func TestModelRendersGHUnauthenticatedError(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, false, &git.PRListError{Kind: git.PRListErrorUnauthenticated, Err: errBoom}, nil)

	content := m.View().Content
	if !strings.Contains(content, "not authenticated") || !strings.Contains(content, "gh auth login") {
		t.Fatalf("expected gh-unauthenticated hint, got:\n%s", content)
	}
}

func TestModelRendersClosedPRSection(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, false, nil, []git.ClosedPR{
		{Number: 5, Title: "Merged fix", State: "MERGED", ClosedAt: time.Now()},
		{Number: 6, Title: "Abandoned idea", State: "CLOSED", ClosedAt: time.Now()},
	})

	content := m.View().Content
	if !strings.Contains(content, "Closed (last 2 weeks)") {
		t.Fatalf("expected closed-PR section header, got:\n%s", content)
	}
	if !strings.Contains(content, "Merged fix") || !strings.Contains(content, "Abandoned idea") {
		t.Fatalf("expected closed PR titles, got:\n%s", content)
	}
}

func TestModelRendersRepoNameOnClosedRowsInAllReposMode(t *testing.T) {
	m := NewModelWithScope("/repo", ui.Settings{}, keys.Manager{}, true)
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, false, nil, []git.ClosedPR{
		{Number: 5, Title: "Merged fix", State: "MERGED", ClosedAt: time.Now(), Repo: "acme/widgets"},
	})

	content := m.View().Content
	if !strings.Contains(content, "widgets") {
		t.Fatalf("expected repo short name 'widgets' on closed row, got:\n%s", content)
	}
}

func TestModelRendersClosedSectionEmptyState(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, true, nil, nil)

	content := m.View().Content
	if !strings.Contains(content, "no recently closed PRs") {
		t.Fatalf("expected closed-PR empty state, got:\n%s", content)
	}
}

func TestModelRendersClosedSectionWhenOpenListEmpty(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, false, nil, []git.ClosedPR{
		{Number: 5, Title: "Merged fix", State: "MERGED", ClosedAt: time.Now()},
	})

	content := m.View().Content
	if !strings.Contains(content, "no PRs found") {
		t.Fatalf("expected open-list empty state, got:\n%s", content)
	}
	if !strings.Contains(content, "Merged fix") {
		t.Fatalf("expected closed section to render alongside empty open list, got:\n%s", content)
	}
}

func TestModelRendersClosedSectionWhenOpenListErrors(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, false, errBoom, []git.ClosedPR{
		{Number: 5, Title: "Merged fix", State: "MERGED", ClosedAt: time.Now()},
	})

	content := m.View().Content
	if !strings.Contains(content, "boom") {
		t.Fatalf("expected open-list error, got:\n%s", content)
	}
	if !strings.Contains(content, "Merged fix") {
		t.Fatalf("expected closed section to render alongside erroring open list, got:\n%s", content)
	}
}

func TestOpenSelectedReturnsOpenURLCmd(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		{Number: 12, Title: "Add widget", URL: "https://example.com/pull/12", UpdatedAt: time.Now()},
	}, true, nil, nil)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd from enter")
	}
	msg := cmd()
	if _, ok := msg.(gotoPRMsg); !ok {
		t.Fatalf("expected gotoPRMsg, got %T", msg)
	}
}

func TestNavigationMovesSelectionIntoClosedSection(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		{Number: 1, Title: "Open one", URL: "https://example.com/pull/1", UpdatedAt: time.Now()},
	}, true, nil, []git.ClosedPR{
		{Number: 2, Title: "Closed one", URL: "https://example.com/pull/2", State: "MERGED", ClosedAt: time.Now()},
		{Number: 3, Title: "Closed two", URL: "https://example.com/pull/3", State: "MERGED", ClosedAt: time.Now()},
	})

	m = sendModel(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.list.Selected() != 1 {
		t.Fatalf("expected selection to move onto first closed row (index 1), got %d", m.list.Selected())
	}

	m = sendModel(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.list.Selected() != 2 {
		t.Fatalf("expected selection to move onto second closed row (index 2), got %d", m.list.Selected())
	}

	// Clamped at the end of the combined list.
	m = sendModel(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if m.list.Selected() != 2 {
		t.Fatalf("expected selection to stay clamped at index 2, got %d", m.list.Selected())
	}

	m = sendModel(m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.list.Selected() != 1 {
		t.Fatalf("expected selection to move back up to index 1, got %d", m.list.Selected())
	}
}

func TestOpenSelectedReturnsClosedURLCmd(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		{Number: 1, Title: "Open one", URL: "https://example.com/pull/1", UpdatedAt: time.Now()},
	}, true, nil, []git.ClosedPR{
		{Number: 2, Title: "Closed one", URL: "https://example.com/pull/2", State: "MERGED", ClosedAt: time.Now()},
	})

	m = sendModel(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a cmd from enter on a closed row")
	}
	msg := cmd()
	goto_, ok := msg.(gotoPRMsg)
	if !ok {
		t.Fatalf("expected gotoPRMsg, got %T", msg)
	}
	if goto_.url != "https://example.com/pull/2" {
		t.Fatalf("expected closed PR's URL, got %q", goto_.url)
	}
}

func TestQuestionMarkOpensHelpOverlay(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.help.IsOpen {
		t.Fatal("help should start closed")
	}

	m = sendModel(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if !m.help.IsOpen {
		t.Fatal("expected help open after ?")
	}

	content := m.View().Content
	if !strings.Contains(content, "Keybindings") {
		t.Fatalf("expected help overlay with Keybindings title, got:\n%s", content)
	}
}

func TestCKeyOnOpenPROpensCommentsPopupAndFetches(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, []git.PR{
		{Number: 12, Title: "Add widget", Repo: "acme/widgets", UpdatedAt: time.Now()},
	}, true, nil, nil)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatal("expected a fetch cmd from c on an open PR")
	}
	m = sendModel(m, tea.KeyPressMsg{Code: 'c', Text: "c"})
	if !m.comments.isOpen || !m.comments.loading {
		t.Fatal("expected comments popup open and loading immediately after c")
	}

	now := time.Now()
	m = sendModel(m, commentsLoadedMsg{comments: []git.PRComment{
		{Author: "alice", Body: "looks good", CreatedAt: now},
	}})
	if m.comments.loading {
		t.Fatal("expected loading to clear once comments arrive")
	}

	content := m.View().Content
	if !strings.Contains(content, "Comments") || !strings.Contains(content, "alice") || !strings.Contains(content, "looks good") {
		t.Fatalf("expected popup with author and body, got:\n%s", content)
	}
}

func TestCKeyOnClosedPRIsNoOp(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = loadPRs(m, nil, false, nil, []git.ClosedPR{
		{Number: 5, Title: "Merged fix", State: "MERGED", ClosedAt: time.Now()},
	})
	m = m.navigateSelection(1) // move onto the closed row

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd != nil {
		t.Fatal("expected no cmd from c on a closed PR row")
	}
	m = sendModel(m, tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m.comments.isOpen {
		t.Fatal("expected comments popup to stay closed for a closed-PR row")
	}
}

func TestCommentsPopupDismissesViaEscQEnter(t *testing.T) {
	for _, key := range []string{"esc", "q", "enter"} {
		t.Run(key, func(t *testing.T) {
			m := NewModel("/repo", ui.Settings{}, keys.Manager{})
			m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
			m = loadPRs(m, []git.PR{
				{Number: 12, Title: "Add widget", Repo: "acme/widgets", UpdatedAt: time.Now()},
			}, true, nil, nil)
			m = sendModel(m, tea.KeyPressMsg{Code: 'c', Text: "c"})
			if !m.comments.isOpen {
				t.Fatal("expected popup open before dismiss")
			}

			var code rune
			switch key {
			case "esc":
				code = tea.KeyEscape
			case "enter":
				code = tea.KeyEnter
			default:
				code = 'q'
			}
			m = sendModel(m, tea.KeyPressMsg{Code: code, Text: key})
			if m.comments.isOpen {
				t.Fatalf("expected popup closed after %q", key)
			}
		})
	}
}

func TestRKeyTriggersRefresh(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if cmd == nil {
		t.Fatal("expected a refresh cmd from R")
	}
	sawOpen, sawClosed := containsMsgTypes(resolveMsgs(cmd))
	if !sawOpen || !sawClosed {
		t.Fatalf("expected R to refresh both sections, sawOpen=%v sawClosed=%v", sawOpen, sawClosed)
	}
}

func TestAKeyTogglesAllReposScopeAndTriggersRefetch(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.allRepos {
		t.Fatal("expected allRepos false by default")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected a refetch cmd from a")
	}
	sawOpen, sawClosed := containsMsgTypes(resolveMsgs(cmd))
	if !sawOpen || !sawClosed {
		t.Fatalf("expected toggling all-repos to refresh both sections, sawOpen=%v sawClosed=%v", sawOpen, sawClosed)
	}
	if !m.allRepos {
		t.Fatal("expected allRepos true after pressing a")
	}

	content := m.View().Content
	if !strings.Contains(content, "all repos") {
		t.Fatalf("expected all-repos indicator in panel, got:\n%s", content)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	if m.allRepos {
		t.Fatal("expected allRepos false after toggling again")
	}
}

func TestNewModelWithScopeStartsAllRepos(t *testing.T) {
	m := NewModelWithScope("/repo", ui.Settings{}, keys.Manager{}, true)
	if !m.allRepos {
		t.Fatal("expected NewModelWithScope(..., true) to start allRepos scoped")
	}
}

func TestRefreshMenuChordTriggersRefresh(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m = sendModel(m, tea.KeyPressMsg{Code: 'm', Text: "m"})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("expected a refresh cmd from m r")
	}
	sawOpen, sawClosed := containsMsgTypes(resolveMsgs(cmd))
	if !sawOpen || !sawClosed {
		t.Fatalf("expected m r to refresh both sections, sawOpen=%v sawClosed=%v", sawOpen, sawClosed)
	}
}

func TestOnPageActivatedTriggersRefetch(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	cmd := m.OnPageActivated()
	if cmd == nil {
		t.Fatal("expected OnPageActivated to return a refetch cmd")
	}
	sawOpen, sawClosed := containsMsgTypes(resolveMsgs(cmd))
	if !sawOpen || !sawClosed {
		t.Fatalf("expected tab-switch-in to refresh both sections, sawOpen=%v sawClosed=%v", sawOpen, sawClosed)
	}
}

// TestInitBatchesOpenAndClosedLoad locks in that the two fetches run
// concurrently (via tea.Batch) rather than one waiting on the other — see
// issues/09-load-time-batched-fetch.md.
func TestInitBatchesOpenAndClosedLoad(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return a cmd")
	}

	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected Init cmd to produce a tea.BatchMsg, got %T", cmd())
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 batched cmds, got %d", len(batch))
	}

	var sawOpen, sawClosed bool
	for _, sub := range batch {
		switch sub().(type) {
		case openPRsLoadedMsg:
			sawOpen = true
		case closedPRsLoadedMsg:
			sawClosed = true
		}
	}
	if !sawOpen || !sawClosed {
		t.Fatalf("expected batch to include both open and closed load cmds, sawOpen=%v sawClosed=%v", sawOpen, sawClosed)
	}
}

// TestOpenSectionRendersBeforeClosedFetchCompletes verifies the open-PR rows
// render as soon as their own fetch completes, without waiting on the
// closed-PR fetch (issues/09-load-time-batched-fetch.md).
func TestOpenSectionRendersBeforeClosedFetchCompletes(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, openPRsLoadedMsg{prs: []git.PR{
		{Number: 12, Title: "Add widget", UpdatedAt: time.Now()},
	}, anyPRs: true})

	content := m.View().Content
	if !strings.Contains(content, "#12") || !strings.Contains(content, "Add widget") {
		t.Fatalf("expected open PR row to render without closed fetch, got:\n%s", content)
	}
	if !strings.Contains(content, "loading") {
		t.Fatalf("expected closed section to still show loading, got:\n%s", content)
	}
}

// TestClosedSectionRendersBeforeOpenFetchCompletes verifies the closed-PR
// section populates independently, even when the open-PR fetch hasn't
// completed yet (issues/09-load-time-batched-fetch.md).
func TestClosedSectionRendersBeforeOpenFetchCompletes(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sendModel(m, closedPRsLoadedMsg{closedPRs: []git.ClosedPR{
		{Number: 5, Title: "Merged fix", State: "MERGED", ClosedAt: time.Now()},
	}})

	content := m.View().Content
	if !strings.Contains(content, "Merged fix") {
		t.Fatalf("expected closed PR row to render without open fetch, got:\n%s", content)
	}
	if !strings.Contains(content, "loading") {
		t.Fatalf("expected open list to still show loading, got:\n%s", content)
	}
}

func TestQKeyReturnsBackCmd(t *testing.T) {
	m := NewModel("/repo", ui.Settings{}, keys.Manager{})
	m = sendModel(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected a nav.Back() cmd from q")
	}
}

var errBoom = errTest("boom")

type errTest string

func (e errTest) Error() string { return string(e) }
