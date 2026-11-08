package prompt

import (
	"cmp"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/sound"
)

// AutoQueueDeadline is when the session's auto-queue countdown ends; zero with
// none running. The countdowns are separate state from auto-advance's.
func (p *Prompt) AutoQueueDeadline(id string) time.Time { return p.autoQueues.deadline(id) }

// mapOf splits a Design session's ticket into its project and map name.
func mapOf(ticket string) (project, mapName string) {
	epic, _, _ := strings.Cut(ticket, "/")
	project, mapName, _ = strings.Cut(epic, ":")
	return project, mapName
}

// DesignReachedBranch starts the countdown on the map's newest Design session
// when that session's switch is on. A readiness failure starts nothing.
func (p *Prompt) DesignReachedBranch(project, mapName string) {
	info, found, err := p.NewestDesignSession(project, mapName)
	if err != nil || !found || !info.AutoQueue {
		return
	}
	if p.startAutoQueue(info.ID, info.Ticket) {
		p.events.Publish(wire.EventSessionChanged, info.ID)
	}
}

// startAutoQueue starts the session's countdown when its map is ready now. It
// starts nothing, and says nothing, when the delay is off, one already runs or
// the readiness check fails.
func (p *Prompt) startAutoQueue(id, ticket string) bool {
	delay := cmp.Or(p.cfg.AutoAdvanceDelay, DefaultAutoAdvanceDelay)
	if delay < 0 || !p.autoQueues.deadline(id).IsZero() {
		return false
	}
	project, mapName := mapOf(ticket)
	if reason, err := p.AutoQueueNotReady(project, mapName); err != nil || reason != "" {
		return false
	}
	c := p.autoQueues.begin(id, time.Now().Add(delay))
	p.sessions.Watches.Start(func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-c.cancel:
			return
		case <-p.sessions.Watches.Stop:
			return
		}
		if !p.autoQueues.finish(id, c) {
			return
		}
		p.events.Publish(wire.EventSessionChanged, id)
		p.queueAtCountdownEnd(id, project, mapName)
	})
	return true
}

// queueAtCountdownEnd re-runs the readiness check, then queues the map's impl
// epic. Any failure queues nothing and names itself in a banner.
func (p *Prompt) queueAtCountdownEnd(id, project, mapName string) {
	implName := mapName + "-impl"
	fail := func(why string) {
		p.log.Info("auto-queue did not queue", "session", id, "epic", implName, "why", why)
		p.sessions.TriggerBanner(id, sound.Attention, "Auto-queue did not queue "+implName+": "+why)
	}
	reason, err := p.AutoQueueNotReady(project, mapName)
	switch {
	case err != nil:
		fail(err.Error())
		return
	case reason != "":
		fail(reason)
		return
	}
	res, err := p.QueueEpic(project, implName)
	switch {
	case err != nil:
		fail(err.Error())
	case res.Refused:
		fail(cmp.Or(res.Message, res.Reason))
	}
}

// cancelAutoQueue stops the session's countdown and tells the person why. It
// reports whether one was running.
func (p *Prompt) cancelAutoQueue(id, why string) bool {
	if !p.autoQueues.stop(id) {
		return false
	}
	p.events.Publish(wire.EventSessionChanged, id)
	p.sessions.TriggerBanner(id, sound.Attention, why)
	return true
}

// cancelAutoQueueRoute is the cancel route: it stops a running countdown and is
// a no-op with none. It leaves the auto-advance countdown alone.
func (p *Prompt) cancelAutoQueueRoute(req wire.SessionRequest) (wire.SessionResult, error) {
	_, meta, res, err := p.sessions.SessionDir(req.ID)
	if res.Refused || err != nil {
		return res, err
	}
	p.cancelAutoQueue(req.ID, "Auto-queue cancelled")
	info := meta.Info()
	if at := p.AutoQueueDeadline(req.ID); !at.IsZero() {
		info.AutoQueueAt = &at
	}
	return wire.SessionResult{Session: &info}, nil
}
