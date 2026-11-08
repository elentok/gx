package prompt

import (
	"fmt"
	"slices"
	"strings"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// EpicQueue is the part of the Queue the auto-queue code uses: the Epic
// page's Queue button operation, and the queued check.
type EpicQueue interface {
	Add(wire.QueueRequest) (wire.QueueResult, error)
	RootQueued(queue.RootRef) bool
}

// epicState is what the readiness rules read about a map and its impl epic.
type epicState struct {
	step   tickets.DesignStep
	held   bool
	impl   tickets.Epic
	queued bool
	ran    bool
}

// notReady returns the first failing condition as text, or "" when the map is
// ready to be queued.
func (s epicState) notReady() string {
	switch {
	case s.step != tickets.StepBranch:
		return fmt.Sprintf("the design is at %q, not at the Branch step", string(s.step))
	case s.held:
		return "the design is held"
	case s.impl.Status == string(schema.StatusDraft):
		return s.impl.Name + " is a draft"
	case s.impl.QueueRefusal() == tickets.QueueRefusalFollowUps:
		return s.impl.Name + " is a follow-ups epic"
	case s.impl.QueueRefusal() == tickets.QueueRefusalNotOpen:
		return s.impl.Name + " is not open"
	case s.queued:
		return s.impl.Name + " is already queued"
	case s.ran:
		return s.impl.Name + " has already run"
	}
	return ""
}

// AutoQueueNotReady decides whether the Design map `project:mapName` may be
// auto-queued. It returns "" when ready, else the failing condition.
func (p *autoQueuer) AutoQueueNotReady(project, mapName string) (string, error) {
	epics, err := p.projects.Epics(project)
	if err != nil {
		return "", err
	}
	implName := mapName + tickets.ImplEpicSuffix
	mapIdx := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == mapName })
	if mapIdx < 0 {
		return "", fmt.Errorf("no map %s:%s", project, mapName)
	}
	s := epicState{step: tickets.DesignStep(epics[mapIdx].DesignStep)}
	implIdx := slices.IndexFunc(epics, func(e tickets.Epic) bool { return e.Name == implName })
	if implIdx < 0 {
		return "there is no " + implName + " epic", nil
	}
	s.impl = epics[implIdx]
	s.held = p.held != nil && p.held(project+":"+mapName)
	s.queued = p.queue != nil && p.queue.RootQueued(queue.RootRef{Project: project, Epic: implName})
	dir, err := p.projects.Dir(project)
	if err != nil {
		return "", err
	}
	s.ran = ralphloop.EpicHasRun(dir, implName)
	return s.notReady(), nil
}

// QueueEpic queues `project:epic` through the Queue's Add, as the Epic page's
// Queue button does. A refusal comes back in the result.
func (p *autoQueuer) QueueEpic(project, epic string) (wire.QueueResult, error) {
	if p.queue == nil {
		return wire.QueueResult{}, fmt.Errorf("no queue")
	}
	return p.queue.Add(wire.QueueRequest{Address: project + ":" + epic})
}

// NewestDesignSession finds the session whose auto-queue switch counts for a
// map: the newest live Design session, else the newest exited one. It mirrors
// designSession in webui/src/design/view.ts.
func (p *autoQueuer) NewestDesignSession(project, mapName string) (wire.SessionInfo, bool, error) {
	metas, err := p.sessions.ListSessionMetas()
	if err != nil {
		return wire.SessionInfo{}, false, err
	}
	var live, exited []wire.SessionInfo
	for _, m := range metas {
		epic, _, _ := strings.Cut(m.Ticket, "/")
		if !m.Design || epic != project+":"+mapName {
			continue
		}
		if info := m.Info(); info.Status.State == wire.SessionExited {
			exited = append(exited, info)
		} else {
			live = append(live, info)
		}
	}
	if len(live) > 0 {
		return slices.MaxFunc(live, func(a, b wire.SessionInfo) int { return a.RunStartedAt.Compare(b.RunStartedAt) }), true, nil
	}
	if len(exited) > 0 {
		return slices.MaxFunc(exited, func(a, b wire.SessionInfo) int { return a.StatusChangedAt.Compare(b.StatusChangedAt) }), true, nil
	}
	return wire.SessionInfo{}, false, nil
}
