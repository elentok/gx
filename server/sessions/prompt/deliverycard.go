package prompt

import (
	"errors"
	"fmt"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/deliver"
	"github.com/elentok/gx/server/wire"
)

// deliveryCard is the card of a send of text that ended unrecovered. Its Kind
// says which of the three it is, so no client matches the reason text. A
// resend (Action) is offered unless the command may already have run.
func deliveryCard(text string, err error) wire.SessionActionable {
	a := wire.SessionActionable{Kind: wire.DeliverySendFailed, Reason: err.Error(), Action: []string{text}, CopyText: text, GxRaised: true}
	switch {
	case errors.Is(err, deliver.ErrMaybeUnsent):
		a.Kind, a.Action = wire.DeliveryMaybeSent, nil
		a.Reason = fmt.Sprintf("gx could not tell if %s ran. The agent's input still shows it. See the screen. gx does not resend: the command could run twice.", text)
	case errors.Is(err, agentrunner.ErrUnknownSkill):
		a.Kind = wire.DeliveryUnknownCommand
		var unknown *agentrunner.UnknownCommandError
		if errors.As(err, &unknown) {
			a.Reason = unknown.Marker + ". It did not run."
		} else {
			a.Reason = err.Error() + ". It did not run."
		}
	}
	return a
}
