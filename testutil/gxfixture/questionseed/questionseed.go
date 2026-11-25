// Package questionseed seeds a paused ticket's question log: earlier answered
// rounds and the open question, as an agent's `gx tickets ask` writes them.
package questionseed

import "github.com/elentok/gx/tickets/ticketlog"

// Question is the open question of a paused ticket, with its earlier rounds.
type Question struct {
	Summary, Text, Block, Handoff string
	Earlier                       []Round
}

// Round is one answered round. Choice answers a choice question; Reply answers a
// text one. Note rides on the answer.
type Round struct {
	Summary, Text, Block, Handoff string
	Choice, Reply, Note           string
}

// Seed appends each earlier round (a question, then its answer) and last the
// open question to the ticket's log. The ticket file stays as it was written.
func Seed(ticketPath string, q Question) error {
	for _, r := range q.Earlier {
		ask, err := ticketlog.AppendLog(ticketPath, question(r.Summary, r.Text, r.Block, r.Handoff))
		if err != nil {
			return err
		}
		ans := ticketlog.LogEvent{
			Type: ticketlog.EventPauseAnswer, Source: ticketlog.SourceWeb, How: ticketlog.HowAnswer,
			Question: ask.Seq, Choice: r.Choice, Note: r.Note,
		}
		if r.Choice == "" {
			ans.Text = r.Reply
		}
		if _, err := ticketlog.AppendLog(ticketPath, ans); err != nil {
			return err
		}
	}
	_, err := ticketlog.AppendLog(ticketPath, question(q.Summary, q.Text, q.Block, q.Handoff))
	return err
}

func question(summary, text, block, handoff string) ticketlog.LogEvent {
	return ticketlog.LogEvent{
		Type: ticketlog.EventPauseQuestion, By: "agent",
		Summary: summary, Text: text, Block: block, Handoff: handoff,
	}
}
