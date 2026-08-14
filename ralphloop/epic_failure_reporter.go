package ralphloop

import (
	"context"
	"time"

	"github.com/elentok/gx/logger"
)

// EpicFailureNotifier is called by the loop registry once a run has
// returned an error, after the run's own EventSink has been closed and
// drained. It is a separate, exported interface (rather than a call
// straight into EventSink) for two reasons: its lifetime must outlive the
// run-scoped sink the drain just tore down, and callers outside this
// package (ui/tickets' loop registry) need to supply a test double without
// reaching into ralphloop's unexported chatTransport.
type EpicFailureNotifier interface {
	EpicFailed(epicName string, err error)
}

// EpicFailureReporter is the EpicFailureNotifier the loop registry uses in
// production: it holds its own chat transports (constructed once, at run
// start, from the same config the run's chatEventSink decoration used) so
// that its EpicFailed call — made after that decoration's sink has already
// closed and drained — still reaches chat instead of being emitted into a
// closed sink and dropped.
type EpicFailureReporter struct {
	scratchDir string
	targets    []epicFailureTarget

	// gateStatePath overrides NotificationGate's real per-user state-file
	// path (see notificationStateFilePath) with a test-only fixed path,
	// mirroring chatEventSink.gateStatePath - so tests never touch the real
	// ~/.local/state/gx/notifications-state.json. Empty (the production
	// default from NewEpicFailureReporter) means "use the real path".
	gateStatePath string
}

type epicFailureTarget struct {
	style     mrkdwnStyle
	transport chatTransport
}

// NewEpicFailureReporter returns a reporter with no configured targets;
// call AddTelegram/AddSlack for each channel the run's own chat decoration
// was configured with.
func NewEpicFailureReporter(scratchDir string) *EpicFailureReporter {
	return &EpicFailureReporter{scratchDir: scratchDir}
}

// AddTelegram configures the reporter to also send via the Telegram Bot
// API, mirroring NewTelegramEventSink's transport.
func (r *EpicFailureReporter) AddTelegram(botToken, chatID string) {
	r.targets = append(r.targets, epicFailureTarget{
		style:     telegramStyle,
		transport: newTelegramTransport(botToken, chatID, telegramAPIBaseURL),
	})
}

// AddSlack configures the reporter to also send via a Slack webhook,
// mirroring NewSlackEventSink's transport.
func (r *EpicFailureReporter) AddSlack(webhookURL string) {
	r.targets = append(r.targets, epicFailureTarget{
		style:     slackStyle,
		transport: newSlackTransport(webhookURL),
	})
}

// gate runs source through NotificationGate (or, under test, an injected
// fixed state-file path — see gateStatePath), mirroring
// chatEventSink.gate.
func (r *EpicFailureReporter) gate(transport, source string) (GateResult, error) {
	if r.gateStatePath != "" {
		return notificationGateAt(r.gateStatePath, transport, notifyKindEpicFailed, source, time.Now(), true, nil)
	}
	return NotificationGate(transport, notifyKindEpicFailed, source, time.Now(), true, nil)
}

// EpicFailed sends the "epic failed" message to every configured target,
// same fire-and-forget/retry-once/run-log semantics as chatEventSink.send.
// counts is loaded fresh here rather than carried over from a live event,
// since the run that would have carried one has already ended. Each target
// is gated (source "epic:<name>", no parkTicket — there's no ticket to park
// this send against) before it sends; a gate trip still writes its
// bookkeeping but suppresses the send itself.
func (r *EpicFailureReporter) EpicFailed(epicName string, err error) {
	if err == nil {
		return
	}
	counts := loadEpicCounts(r.scratchDir, epicName)
	source := "epic:" + epicName
	for _, target := range r.targets {
		result, gateErr := r.gate(target.transport.name(), source)
		if gateErr != nil {
			logger.Debug("%s: notification gate: %v\n", target.transport.name(), gateErr)
			continue
		}
		if result.Decision != Allowed {
			continue
		}

		text := target.style.epicFailedText(epicName, counts, err.Error())
		sendNotification(r.scratchDir, epicName, target.transport.name(), notifyKindEpicFailed, text.String(), target.transport.timeout(), func(ctx context.Context) error {
			_, err := target.transport.sendSync(ctx, text)
			return err
		}, nil)
	}
}
