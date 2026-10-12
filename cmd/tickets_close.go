package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/spf13/cobra"
)

// followUpsArchivedEpic receives every closed follow-up, so the live outbox
// only holds what still needs a decision.
const followUpsArchivedEpic = "follow-ups-archived"

var closeReasons = []string{"fixed", "obsolete", "duplicate", "not-worth-it"}

// runTicketsClose cancels one draft/open follow-up with a recorded reason and
// moves it into the project's archive epic, in one step. Every refusal runs
// before anything is written.
func runTicketsClose(d deps, addr, reason, note string, w io.Writer) error {
	if !slices.Contains(closeReasons, reason) {
		return &tickets.AddressError{Code: tickets.CodeCloseReason, Msg: fmt.Sprintf("reason %q; want one of %s", reason, strings.Join(closeReasons, ", "))}
	}
	note = strings.TrimSpace(note)
	if reason == "duplicate" && note == "" {
		return &tickets.AddressError{Code: tickets.CodeCloseNoteNeeded, Msg: "a duplicate needs --note naming the kept ticket's address"}
	}
	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	a, src, err := findTicket(cwd, addr)
	if err != nil {
		return err
	}
	if a.Epic != followUpsEpic {
		return &tickets.AddressError{Code: tickets.CodeNotFollowUpsEpic, Msg: fmt.Sprintf("close works on tickets in a %s epic, not %q", followUpsEpic, a.Epic)}
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(src.Path)))

	if err := refuseIfActive(d.dial(), a); err != nil {
		return err
	}

	resolution := fmt.Sprintf("- reason: %s\n", reason)
	if note != "" {
		resolution += fmt.Sprintf("- note: %s\n", note)
	}
	resolution += fmt.Sprintf("- original: %s\n", a)

	created, err := ensureArchiveEpic(root, filepath.Join(root, followUpsEpic))
	if err != nil {
		return err
	}
	p, err := archiveTicket(root, a, src, moveOpts{archive: true, resolution: resolution})
	if err != nil {
		if created {
			_ = os.RemoveAll(filepath.Join(root, followUpsArchivedEpic))
		}
		return err
	}
	pingServer(p.newPath)
	return printMove(w, p, false, "")
}

// runTicketsCloseSweep moves every done or cancelled follow-up of the project
// into the archive, keeping its status. A cancelled one without a Resolution
// gets one naming its original address. An empty project argument means the
// cwd project.
func runTicketsCloseSweep(d deps, project string, w io.Writer) error {
	cwd, err := d.getwd()
	if err != nil {
		return err
	}
	if project == "" {
		cwdRoot, err := ticketRoot(cwd)
		if err != nil {
			return err
		}
		project = tickets.ProjectName(cwdRoot)
	}
	root, err := projectRoot(cwd, project)
	if err != nil {
		return err
	}
	epics, err := tickets.Load(root)
	if err != nil {
		return err
	}
	var swept []tickets.Ticket
	for _, e := range epics {
		if e.Name != followUpsEpic {
			continue
		}
		for _, t := range e.Tickets {
			if t.Status == string(schema.StatusDone) || t.Status == string(schema.StatusCancelled) {
				swept = append(swept, t)
			}
		}
	}
	if len(swept) == 0 {
		return nil
	}
	if _, err := ensureArchiveEpic(root, filepath.Join(root, followUpsEpic)); err != nil {
		return err
	}
	for _, t := range swept {
		a := tickets.Address{Project: project, Epic: followUpsEpic, ID: t.Identifier}
		opts := moveOpts{archive: true, recordOriginal: t.Status == string(schema.StatusCancelled)}
		p, err := archiveTicket(root, a, t, opts)
		if err != nil {
			return err
		}
		pingServer(p.newPath)
		if err := printMove(w, p, false, ""); err != nil {
			return err
		}
	}
	return nil
}

// ensureArchiveEpic creates the archive epic draft when missing, of the same
// kind as the follow-ups epic it archives. It reports whether it created it.
func ensureArchiveEpic(root, followUpsPath string) (bool, error) {
	if info, err := os.Stat(filepath.Join(root, followUpsArchivedEpic)); err == nil && info.IsDir() {
		return false, nil
	}
	kind := tickets.KindLoop
	if tickets.IsMapEpic(followUpsPath) {
		kind = tickets.KindMap
	}
	_, err := tickets.CreateEpic(root, followUpsArchivedEpic, tickets.EpicFields{Kind: kind, Status: string(schema.StatusDraft)}, "")
	return err == nil, err
}

// archiveTicket moves src into the archive epic under both epic locks.
func archiveTicket(root string, a tickets.Address, src tickets.Ticket, opts moveOpts) (movePlan, error) {
	srcEpicPath := filepath.Join(root, a.Epic)
	unlock, err := lockEpics(srcEpicPath, filepath.Join(root, followUpsArchivedEpic))
	if err != nil {
		return movePlan{}, err
	}
	defer unlock()

	p, err := planMove(root, a, src, followUpsArchivedEpic, "", opts)
	if err != nil {
		return movePlan{}, err
	}
	return p, applyMove(p)
}

func newTicketsCloseCmd(d deps) *cobra.Command {
	var reason, note string
	var sweep, sweepDone bool
	closeCmd := &cobra.Command{
		Use:   "close <addr> --reason fixed|obsolete|duplicate|not-worth-it [--note ...]",
		Short: "cancel a draft/open follow-up with a reason and archive it; --sweep moves done and cancelled follow-ups into the archive",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(c *cobra.Command, args []string) error {
			if sweep || sweepDone {
				if reason != "" || note != "" {
					return fmt.Errorf("--sweep takes no --reason or --note")
				}
				project := ""
				if len(args) == 1 {
					project = args[0]
				}
				return runTicketsCloseSweep(d, project, c.OutOrStdout())
			}
			if len(args) != 1 {
				return fmt.Errorf("close needs one ticket address")
			}
			return runTicketsClose(d, args[0], reason, note, c.OutOrStdout())
		},
	}
	closeCmd.Flags().StringVar(&reason, "reason", "", "fixed, obsolete, duplicate or not-worth-it")
	closeCmd.Flags().StringVar(&note, "note", "", "free text; for duplicate, the kept ticket's address")
	closeCmd.Flags().BoolVar(&sweep, "sweep", false, "move every done or cancelled follow-up of the project (the optional argument, default the cwd's) into the archive")
	// --done is the flag's old name, kept so existing skill text and habits work.
	closeCmd.Flags().BoolVar(&sweepDone, "done", false, "alias of --sweep")
	_ = closeCmd.Flags().MarkHidden("done")
	return closeCmd
}
