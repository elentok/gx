package rowactions

import (
	"context"
	"strings"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

// recoverPrefix marks a menu pick that runs one of detail.actions; the rest of
// the action is the verb's word.
const RecoverPrefix = "recover:"

const RecommendedMark = " (Recommended)"

// RecoveryAction is one detail.actions entry as the menu shows it.
type RecoveryAction struct {
	Word, Label, Help string
	Recommended       bool
}

// recoveryActions reads the server's verbs with the label and tooltip the
// server sent for each. A nil detail offers none.
func RecoveryActions(d *wire.TicketDetail) []RecoveryAction {
	if d == nil {
		return nil
	}
	out := make([]RecoveryAction, 0, len(d.Actions))
	for _, word := range d.Actions {
		info := d.ActionInfo[word]
		if info.Label == "" {
			info.Label = word
		}
		out = append(out, RecoveryAction{
			Word: word, Label: info.Label, Help: info.Tooltip,
			Recommended: d.Recommendation != nil && d.Recommendation.Verb == word,
		})
	}
	return out
}

// ticketDetail reads address's detail. With no server it returns nil and no
// error; a failed read returns the error, and the menu opens without recovery
// items.
func TicketDetail(api tabapi.ReadsAPI, address string) (*wire.TicketDetail, error) {
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
func RecoveryWord(action string) (word string, ok bool) {
	return strings.CutPrefix(action, RecoverPrefix)
}
