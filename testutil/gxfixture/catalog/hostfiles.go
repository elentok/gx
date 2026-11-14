package catalog

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// hostFilesScenario is demo with a chat session whose messages link files, and
// one ticket whose body names paths; the repo files, worktree and mock page are
// laid out by the fixture program (HostFiles).
func hostFilesScenario() scenario {
	return scenario{
		HostFiles: true,
		Chat:      true,
		Epics: []epic{{Project: project, Name: spec.HostFilesEpic, Tickets: []ticket{
			{ID: spec.HostFilesTicket, Slug: "links", Opts: servertest.TicketOpts{}},
		}}},
		Sessions: []sessionSpec{{Project: project, Prompt: "hello", Read: true, HostFilesChat: true}},
	}
}
