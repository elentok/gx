package server

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ralphloop"
)

const (
	// NoticeDiskClean is the notify kind for the end of an automatic clean.
	NoticeDiskClean = "disk-clean"

	// defaultCleanTimeout is a constant, not a setting: the spec does not make it one.
	defaultCleanTimeout = 10 * time.Minute

	noCleanCommand = "no cleanup command set"
)

// cleanCmd is one cleanup command of a disk and the directory it runs in.
type cleanCmd struct {
	argv []string
	dir  string
}

func (c cleanCmd) String() string { return strings.Join(c.argv, " ") }

// cleanEnd marks an observe call as the end of a clean. failed names the first
// failed command; empty means every command succeeded.
type cleanEnd struct{ failed string }

// cleanWork is the distinct (argv, repo) pairs of every project that watches
// the disk and has a cleanup command, in a stable order. It reads the settings
// and project files now, so a reload applies to the next clean.
func (s *Server) cleanWork(projects []string) []cleanCmd {
	base := config.Config{DiskSpace: s.settings.Load().DiskSpace}
	var work []cleanCmd
	seen := map[string]bool{}
	for _, name := range projects {
		dir, err := s.projectDir(name)
		if err != nil {
			continue
		}
		pf, err := config.ReadProjectFile(dir)
		if err != nil || pf.Repo == nil {
			continue
		}
		argv := base.WithProject(pf).DiskSpace.CleanupCommand
		if len(argv) == 0 {
			continue
		}
		c := cleanCmd{argv: slices.Clone(argv), dir: *pf.Repo}
		if key := c.dir + "\x00" + strings.Join(c.argv, "\x00"); !seen[key] {
			seen[key] = true
			work = append(work, c)
		}
	}
	slices.SortFunc(work, func(a, b cleanCmd) int {
		return strings.Compare(a.dir+"\x00"+a.String(), b.dir+"\x00"+b.String())
	})
	return work
}

// startClean runs the disk's cleanup work in the background. The caller has
// marked the clean spent and running. A disk with no work ends at once with a
// notice that says so.
func (s *Server) startClean(id, probe string, projects []string) {
	work := s.cleanWork(projects)
	if len(work) == 0 {
		s.noCleanWork(id)
		return
	}
	s.events.publish(EventDiskWarnChanged, "")
	s.goSettle(func() {
		s.endClean(id, probe, s.runCleanWork(work))
	})
}

// noCleanWork ends a clean that has nothing to run: no run, no running flag.
func (s *Server) noCleanWork(id string) {
	d := &s.disks
	d.mu.Lock()
	st := d.disks[id]
	st.cleaning = false
	st.lastClean = noCleanCommand
	name, free, held := st.reading.Name, st.reading.Free, st.held
	d.mu.Unlock()
	s.events.publish(EventDiskWarnChanged, "")
	s.sendCleanNotice(name, noCleanCommand, free, held)
}

// runCleanWork runs every command, each under its own time limit, and returns
// the first failed command ("" when none failed).
func (s *Server) runCleanWork(work []cleanCmd) (failed string) {
	limit := s.cfg.CleanupTimeout
	if limit <= 0 {
		limit = defaultCleanTimeout
	}
	run := s.cfg.CleanupRunner
	if run == nil {
		run = RunCleanupCommand
	}
	for _, c := range work {
		ctx, cancel := context.WithTimeout(context.Background(), limit)
		out, err := run(ctx, c.argv, c.dir)
		timedOut := ctx.Err() != nil
		cancel()
		if err == nil {
			continue
		}
		code := "timed out"
		if !timedOut {
			code = "error: " + err.Error()
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				code = fmt.Sprintf("exit code %d", exit.ExitCode())
			}
		}
		s.log.Warn("disk cleanup command failed", "command", c.String(), "dir", c.dir, "result", code,
			"output", ralphloop.OutputTail(out))
		if failed == "" {
			failed = c.String()
		}
	}
	return failed
}

// RunCleanupCommand is the default CleanupRunner. It is exported so a test
// whose harness fakes the runner can opt back in to the real one.
func RunCleanupCommand(ctx context.Context, argv []string, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	// A killed command's children can keep the output pipe open; don't wait on them.
	cmd.WaitDelay = time.Second
	return cmd.CombinedOutput()
}

// endClean ends every clean: it reads the disk again and lets observe set the
// hold from that reading.
func (s *Server) endClean(id, probe, failed string) {
	r, err := s.freeSpace(probe)
	if err != nil {
		s.log.Warn("read free space", "path", probe, "err", err)
		d := &s.disks
		d.mu.Lock()
		r = d.disks[id].reading
		d.mu.Unlock()
	}
	s.observe(r, probe, nil, time.Now(), &cleanEnd{failed: failed})
}

// sendCleanNotice sends the one notice of a clean.
func (s *Server) sendCleanNotice(disk, outcome string, free uint64, held bool) {
	state := "dispatch is not held"
	if held {
		state = "dispatch is held"
	}
	title := "disk clean finished on " + disk
	if strings.HasPrefix(outcome, "failed") {
		title = "disk clean failed on " + disk
	}
	s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeDiskClean, Emoji: "🧹", Title: title,
		Detail: outcome + "; " + humanize.Bytes(free) + " free now; " + state})
}
