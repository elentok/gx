package tickets

import (
	"context"
	"strings"

	"github.com/elentok/gx/server"
)

// recoverPrefix marks a menu pick that runs one of detail.actions; the rest of
// the action is the verb's word.
const recoverPrefix = "recover:"

const recommendedMark = " (Recommended)"

// recoveryAction is one detail.actions entry as the menu shows it.
type recoveryAction struct {
	word, label, help string
	recommended       bool
}

// recoveryActions reads the server's verbs with the label and tooltip the
// server sent for each. A nil detail offers none.
func recoveryActions(d *server.TicketDetail) []recoveryAction {
	if d == nil {
		return nil
	}
	out := make([]recoveryAction, 0, len(d.Actions))
	for _, word := range d.Actions {
		info := d.ActionInfo[word]
		if info.Label == "" {
			info.Label = word
		}
		out = append(out, recoveryAction{
			word: word, label: info.Label, help: info.Tooltip,
			recommended: d.Recommendation != nil && d.Recommendation.Verb == word,
		})
	}
	return out
}

// ticketDetail reads address's detail. With no server it returns nil and no
// error; a failed read returns the error, and the menu opens without recovery
// items.
func ticketDetail(api ServerAPI, address string) (*server.TicketDetail, error) {
	if api == nil {
		return nil, nil
	}
	d, err := api.Detail(context.Background(), address)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// recoveryWord is the verb of a recovery pick; ok is false for any other action.
func recoveryWord(action string) (word string, ok bool) {
	return strings.CutPrefix(action, recoverPrefix)
}
