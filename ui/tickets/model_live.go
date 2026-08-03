package tickets

// syncRunSnapshot replaces presentation state atomically from the registry's
// durable projection; it never reads the producer stream.
func (m *Model) syncRunSnapshot(epicName string) {
	snapshot, ok := ralphLoopRegistry.runSnapshot(epicName)
	if !ok {
		return
	}
	if m.live == nil {
		m.live = map[string]map[string]liveTicketState{}
	}
	if m.labelIdentifier == nil {
		m.labelIdentifier = map[string]map[string]string{}
	}
	live := make(map[string]liveTicketState, len(snapshot.Tickets))
	labels := make(map[string]string, len(snapshot.Tickets))
	for identifier, ticket := range snapshot.Tickets {
		live[identifier] = liveTicketState{
			running:   ticket.Running,
			paused:    ticket.Paused,
			label:     ticket.Label,
			pauseKind: ticket.PauseKind,
			reason:    ticket.PauseReason,
			phase:     livePhaseImplementing,
			tokens:    ticket.ContextTokens,
		}
		labels[ticket.Label] = identifier
	}
	m.live[epicName] = live
	m.labelIdentifier[epicName] = labels
	if m.implementingEpics == nil {
		m.implementingEpics = map[string]bool{}
	}
	m.implementingEpics[epicName] = snapshot.State == RunStateRunning
}

// clearLiveTracking resets m.implementEpic's live-orchestrator state — kept
// as this zero-arg convenience for call sites/tests that only ever track one
// epic at a time; a poll/sync learning of a *specific* epic's finish (which
// might not be m.implementEpic once more than one epic is tracked, ticket 05)
// goes through clearLiveTrackingFor(epicName) directly so it can't wipe a
// different, still-running epic's live state.
func (m *Model) clearLiveTracking() {
	m.clearLiveTrackingFor(m.implementEpic)
}

// clearLiveTrackingFor removes epicName's live-orchestrator state once its
// run has finished (or once this Model learns of a finish it missed while
// backgrounded — see handleImplementSync), reverting that epic's ticket/epic
// rendering to the normal disk-based view.
func (m *Model) clearLiveTrackingFor(epicName string) {
	delete(m.live, epicName)
	delete(m.labelIdentifier, epicName)
	delete(m.implementingEpics, epicName)
	if m.implementEpic == epicName {
		m.implementEpic = ""
	}
}
