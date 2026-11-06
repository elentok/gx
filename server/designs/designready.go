package designs

import (
	"path/filepath"

	"github.com/elentok/gx/tickets"
)

// readySeedMark is taken once, by the first scan of a server that has never
// run this code. That scan marks the Designs already at Branch or Done as
// sent, so installing the version does not announce old Designs.
const readySeedMark = "design-ready:seeded"

func readyMark(project, implEpic string) string { return "design-ready:" + project + "/" + implEpic }

// ticketCounts maps each epic's directory name to its ticket count.
func ticketCounts(epics []tickets.Epic) map[string]int {
	counts := map[string]int{}
	for _, e := range epics {
		counts[filepath.Base(e.Path)] = len(e.Tickets)
	}
	return counts
}

// sendReady sends the Ready message for a map that just moved to Branch, when
// its impl epic exists and has no mark yet. A mark that cannot be taken sends
// nothing: a message may be lost, never doubled. The mark keeps it once across
// restarts, as the first scan of a run treats every Design as moved.
func (s *Designs) sendReady(project, epic string, counts map[string]int) {
	impl := epic + tickets.ImplEpicSuffix
	n, ok := counts[impl]
	if !ok {
		return
	}
	isNew, err := s.chat.MarkOnce(readyMark(project, impl))
	if err != nil {
		s.log.Warn("design ready mark", "map", project+":"+epic, "err", err)
		return
	}
	if isNew {
		s.chat.DesignReady(project, s.projects.ChatOverride(project), epic, impl, n, s.epicLink(project, impl))
	}
}

// readySeeded reports whether this scan may send. The first scan ever (the
// seed mark is new) marks every Design at Branch or Done and sends nothing, so
// it reports false. An unreadable mark store also reports false.
func (s *Designs) readySeeded() bool {
	if s.seeded.Load() {
		return true
	}
	isNew, err := s.chat.MarkOnce(readySeedMark)
	if err != nil {
		s.log.Warn("design ready seed mark", "err", err)
		return false
	}
	if !isNew {
		s.seeded.Store(true)
		return true
	}
	s.seedReady()
	s.seeded.Store(true)
	return false
}

// seedReady marks every existing Design at Branch or Done as sent.
func (s *Designs) seedReady() {
	dirs, err := tickets.ProjectDirs(s.ticketStore)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		epics, _ := s.idx.EpicsOf(dir)
		for _, e := range epics {
			if e.IsMap && pastBranchStep(tickets.DesignStep(tickets.LoadEpicTicketMD(e.Path).DesignStep)) {
				if _, err := s.chat.MarkOnce(readyMark(tickets.ProjectName(dir), filepath.Base(e.Path)+tickets.ImplEpicSuffix)); err != nil {
					s.log.Warn("design ready seed", "err", err)
				}
			}
		}
	}
}

// epicLink is the epic's page; empty when the web listener is off.
func (s *Designs) epicLink(project, epic string) string {
	base := s.webURL()
	if base == "" {
		return ""
	}
	return base + "/#/e/" + project + ":" + epic
}
