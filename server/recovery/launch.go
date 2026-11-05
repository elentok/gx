package recovery

import (
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

const (
	implementSkill   = "gx-implement"
	codeReviewSkill  = "gx-code-review"
	verifySkill      = "gx-verify-epic"
	oneOffSkill      = "gx-one-off"
	evolveSkill      = "gx-evolve"
	investigateSkill = "gx-investigate"
	// mergeSkill carries its flag: the prompt appends the ticket address to it.
	mergeSkill = "gx-merge --ticket"
)

// LaunchSkill is the skill an agent starts under: code-review tickets get their
// own, since gx-implement would ask them for commits, prompt tickets (the
// one-offs) get one that keeps the TDD rules out of the way, and investigate
// and diagnose tickets run gx-investigate's unattended mode.
func LaunchSkill(t tickets.Ticket) string {
	switch {
	case t.IsCodeReview():
		return codeReviewSkill
	case t.IsVerify():
		return verifySkill
	case t.Type == string(schema.TypePrompt), t.Type == string(schema.TypeTrunkConflict):
		return oneOffSkill
	case t.Type == string(schema.TypeEvolve):
		return evolveSkill
	case t.Type == string(schema.TypeInvestigate), t.Type == string(schema.TypeDiagnose):
		return investigateSkill
	case t.Type == string(schema.TypeMerge):
		return mergeSkill
	}
	return implementSkill
}

// LaunchPrompt is the first prompt of an agent: the skill command, then the note.
func LaunchPrompt(agent ralphloop.AgentKind, skill, note, address string) string {
	prompt := ralphloop.SkillPrompt(agent, skill, address)
	if note != "" {
		prompt += "\n\n" + note
	}
	return prompt
}
