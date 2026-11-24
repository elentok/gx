package notify

import "time"

// ChannelCap counts one attempt on a Desktop, Web toast or Push channel and
// says what the channel does with it. Call it last, after the switch, the kind
// list and every other gate: only a notification that would be delivered is an
// attempt.
//
// send is true when the notification goes out. trip is true on the one call
// that trips the breaker: that call is dropped too, and the channel then sends
// its notice. A tripped channel drops every attempt, and each one still counts,
// so a flood keeps it tripped until the channel has had no attempt for clear
// (zero means seriesRetention).
//
// Unlike the Chat gate it never touches a ticket: the per-source mute and the
// needs-repair parking stay with Telegram and Slack. If the state file is
// unavailable the notification goes out.
func ChannelCap(path, channel string, now time.Time, clear time.Duration) (send, trip bool) {
	if path == "" {
		return true, false
	}
	send = true
	err := updateNotificationStateAt(path, func(state *NotificationState) {
		send, trip = applyChannelCap(state, channel, now, clear)
	})
	if err != nil {
		return true, false
	}
	return send, trip
}

// ChannelTripped reports whether the channel's breaker is still tripped at now.
// It is the one predicate for the cap itself and for any status display.
func (ts TransportState) ChannelTripped(now time.Time, clear time.Duration) bool {
	if !ts.Muted {
		return false
	}
	if clear == 0 {
		clear = seriesRetention
	}
	if len(ts.Sends) == 0 {
		return false
	}
	return now.Sub(ts.Sends[len(ts.Sends)-1].Time) < clear
}

func applyChannelCap(state *NotificationState, channel string, now time.Time, clear time.Duration) (send, trip bool) {
	ts := state.Transports[channel]
	defer func() { state.Transports[channel] = ts }()

	if ts.Muted && !ts.ChannelTripped(now, clear) {
		ts.Muted, ts.TrippedAt, ts.Reason, ts.Sends = false, time.Time{}, "", nil
	}
	ts.Sends = append(trimSends(ts.Sends, now), NotificationSend{Time: now})
	if ts.Muted {
		return false, false
	}
	if countSendsInWindow(ts.Sends, now, globalWindow) < globalThreshold {
		return true, false
	}
	ts.Muted, ts.TrippedAt, ts.Reason = true, now, "auto-trip"
	ts.Trips = append(ts.Trips, TransportTrip{TrippedAt: now, Reason: "auto-trip"})
	return false, true
}
