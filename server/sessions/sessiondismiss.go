package sessions

import (
	"fmt"

	"github.com/elentok/gx/server/wire"
)

// DismissSession clears a delivery-failure card. It judges the saved meta, not
// Info(), so a card replaced since the caller's read is not cleared by mistake;
// Info() is consulted only to tell a derived or blocked card from no card.
func (mg *Manager) DismissSession(req wire.SessionRequest) (wire.SessionResult, error) {
	dir, _, res, err := mg.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	var info wire.SessionInfo
	var refusal wire.SessionResult
	if err := mg.UpdateSessionMeta(dir, req.ID, func(m *Meta) bool {
		info = m.Info()
		switch {
		case m.Actionable.IsDelivery():
			m.Actionable = wire.SessionActionable{}
			info = m.Info()
			return true
		case info.Actionable.Reason != "":
			refusal = Refusal(wire.ReasonSessionNotDeliveryCard, fmt.Sprintf("session %s shows a different card; answer or retry it instead of dismissing it", req.ID))
		default:
			refusal = Refusal(wire.ReasonSessionNoCard, fmt.Sprintf("session %s has no card to dismiss", req.ID))
		}
		return false
	}); err != nil {
		return wire.SessionResult{}, err
	}
	if refusal.Refused {
		return refusal, nil
	}
	return wire.SessionResult{Session: &info}, nil
}
