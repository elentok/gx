package tickets

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/cmd/ticketref"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/ticketops"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
	"github.com/elentok/gx/tickets/ticketmove"
	"github.com/spf13/cobra"
)

var closeReasons = []string{"fixed", "obsolete", "duplicate", "not-worth-it"}

// runTicketsClose cancels one draft/open follow-up with a recorded reason and
// moves it into the project's archive epic, in one step. Every refusal runs
// before anything is written.
func runTicketsClose(d Deps, addr, reason, note string, w io.Writer) error {
	if !slices.Contains(closeReasons, reason) {
		return &tickets.AddressError{Code: tickets.CodeCloseReason, Msg: fmt.Sprintf("reason %q; want one of %s", reason, strings.Join(closeReasons, ", "))}
	}
	note = strings.TrimSpace(note)
	if reason == "duplicate" && note == "" {
		return &tickets.AddressError{Code: tickets.CodeCloseNoteNeeded, Msg: "a duplicate needs --note naming the kept ticket's address"}
	}
	cwd, err := d.Getwd()
	if err != nil {
		return err
	}
	a, src, err := ticketref.FindTicket(d.dirs(), cwd, addr)
	if err != nil {
		return err
	}
	if a.Epic != tickets.FollowUpsEpic {
		return &tickets.AddressError{Code: tickets.CodeNotFollowUpsEpic, Msg: fmt.Sprintf("close works on tickets in a %s epic, not %q", tickets.FollowUpsEpic, a.Epic)}
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(src.Path)))

	if err := refuseIfActive(d.dirs(), d.Dial(), a); err != nil {
		return err
	}

	resolution := fmt.Sprintf("- reason: %s\n", reason)
	if note != "" {
		resolution += fmt.Sprintf("- note: %s\n", note)
	}
	resolution += tickets.OriginalLine(a) + "\n"

	runEvents, _, err := runlog.ReadEvents(root, a.Epic)
	if err != nil {
		return err
	}
	log, err := tickets.ReadLog(src.Path)
	if err != nil {
		return err
	}
	friction := ticketops.Friction(a.ID, runEvents, log)

	created, err := ensureArchiveEpic(d.dirs(), root, filepath.Join(root, tickets.FollowUpsEpic))
	if err != nil {
		return err
	}
	p, err := archiveTicket(root, a, src, ticketmove.MoveOpts{Archive: true, Resolution: resolution, Friction: &friction})
	if err != nil {
		if created {
			_ = os.RemoveAll(filepath.Join(root, tickets.ArchivedFollowUpsEpic))
		}
		return err
	}
	PingServer(d.Env, p.NewPath)
	return printMove(w, p, false, "")
}

// runTicketsCloseSweep moves every done or cancelled follow-up of the project
// into the archive, keeping its status. A cancelled one without a Resolution
// gets one naming its original address. An empty project argument means the
// cwd project.
func runTicketsCloseSweep(d Deps, project string, w io.Writer) error {
	root, project, err := ticketref.ResolveProject(d.Env, project)
	if err != nil {
		return err
	}
	epics, err := tickets.Load(root)
	if err != nil {
		return err
	}
	var swept []tickets.Ticket
	for _, e := range epics {
		if e.Name != tickets.FollowUpsEpic {
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
	if _, err := ensureArchiveEpic(d.dirs(), root, filepath.Join(root, tickets.FollowUpsEpic)); err != nil {
		return err
	}
	for _, t := range swept {
		a := tickets.Address{Project: project, Epic: tickets.FollowUpsEpic, ID: t.Identifier}
		opts := ticketmove.MoveOpts{Archive: true, RecordOriginal: t.Status == string(schema.StatusCancelled)}
		p, err := archiveTicket(root, a, t, opts)
		if err != nil {
			return err
		}
		PingServer(d.Env, p.NewPath)
		if err := printMove(w, p, false, ""); err != nil {
			return err
		}
	}
	return nil
}

// ensureArchiveEpic creates the archive epic draft when missing, of the same
// kind as the follow-ups epic it archives. It reports whether it created it.
func ensureArchiveEpic(dirs config.Dirs, root, followUpsPath string) (bool, error) {
	if info, err := os.Stat(filepath.Join(root, tickets.ArchivedFollowUpsEpic)); err == nil && info.IsDir() {
		return false, nil
	}
	records, err := conversations.DirIn(dirs)
	if err != nil {
		return false, err
	}
	kind := tickets.KindLoop
	if tickets.IsMapEpic(followUpsPath) {
		kind = tickets.KindMap
	}
	_, err = tickets.CreateEpic(records, root, tickets.ArchivedFollowUpsEpic, tickets.EpicFields{Kind: kind, Status: string(schema.StatusDraft)}, "")
	return err == nil, err
}

// archiveTicket moves src into the archive epic under both epic locks.
func archiveTicket(root string, a tickets.Address, src tickets.Ticket, opts ticketmove.MoveOpts) (ticketmove.MovePlan, error) {
	srcEpicPath := filepath.Join(root, a.Epic)
	unlock, err := ticketmove.LockEpics(srcEpicPath, filepath.Join(root, tickets.ArchivedFollowUpsEpic))
	if err != nil {
		return ticketmove.MovePlan{}, err
	}
	defer unlock()

	p, err := ticketmove.PlanMove(root, a, src, tickets.ArchivedFollowUpsEpic, "", opts)
	if err != nil {
		return ticketmove.MovePlan{}, err
	}
	return p, ticketmove.ApplyMove(p)
}

func newTicketsCloseCmd(d Deps) *cobra.Command {
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
