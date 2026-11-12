package workspaces

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/elentok/gx/server/wire"
)

var refusals = map[Reason]string{
	ReasonBlankName:    wire.ReasonWorkspaceBlankName,
	ReasonNameTaken:    wire.ReasonWorkspaceNameTaken,
	ReasonUnknown:      wire.ReasonWorkspaceUnknown,
	ReasonDefaultFixed: wire.ReasonWorkspaceDefaultFixed,
	ReasonBadOrder:     wire.ReasonWorkspaceBadOrder,
	ReasonBlankKey:     wire.ReasonWorkspaceBlankKey,
	ReasonUnknownKey:   wire.ReasonWorkspaceUnknownKey,
	ReasonFollowsEpic:  wire.ReasonWorkspaceFollowsEpic,
	ReasonNotJoinable:  wire.ReasonWorkspaceNotJoinable,
	ReasonBadSort:      wire.ReasonWorkspaceBadSort,
}

// Routes lists the workspace endpoints. The getter is called per request, so a
// route can be listed before its Store is built. Listed returns the workspaces
// with their Needs-you counts filled in.
func Routes(get func() *Store, listed func() []wire.Workspace) []wire.Route {
	write := func(pattern string, do func(*Store, wire.WorkspaceRequest) error) wire.Route {
		return wire.Route{Pattern: pattern, Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) {
			var req wire.WorkspaceRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s := get()
			err := do(s, req)
			var reason Reason
			switch {
			case err == nil:
				wire.WriteJSON(w, wire.WorkspaceResult{Workspaces: listed()})
			case errors.As(err, &reason):
				wire.WriteJSON(w, wire.WorkspaceResult{Refused: true, Reason: refusals[reason], Message: reason.Error()})
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}}
	}
	hold := func(pattern string, do func(*Store, wire.HoldRequest) (wire.TargetsResult, error)) wire.Route {
		return wire.Route{Pattern: pattern, Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) {
			var req wire.HoldRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			res, err := do(get(), req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(w, res)
		}}
	}
	return []wire.Route{
		hold("POST /v1/workspaces/hold", func(s *Store, r wire.HoldRequest) (wire.TargetsResult, error) {
			return s.HoldEpics(r.Targets, r.Yes, r.DryRun)
		}),
		hold("POST /v1/workspaces/unhold", func(s *Store, r wire.HoldRequest) (wire.TargetsResult, error) { return s.UnholdEpics(r.Targets) }),
		{Pattern: "POST /v1/workspaces/move", Web: wire.WebWrite, Handler: func(w http.ResponseWriter, r *http.Request) {
			var req wire.WorkspaceRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			res, err := get().MoveTargets(req.Targets, req.Name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wire.WriteJSON(w, res)
		}},
		{Pattern: "GET /v1/workspaces", Web: wire.WebRead, Handler: func(w http.ResponseWriter, _ *http.Request) {
			wire.WriteJSON(w, wire.WorkspaceList{Workspaces: listed()})
		}},
		write("POST /v1/workspaces/create", func(s *Store, r wire.WorkspaceRequest) error { return s.Create(r.Name) }),
		write("POST /v1/workspaces/rename", func(s *Store, r wire.WorkspaceRequest) error { return s.Rename(r.Name, r.To) }),
		write("POST /v1/workspaces/delete", func(s *Store, r wire.WorkspaceRequest) error { return s.Delete(r.Name) }),
		write("POST /v1/workspaces/reorder", func(s *Store, r wire.WorkspaceRequest) error { return s.Reorder(r.Order) }),
		write("POST /v1/workspaces/sort", func(s *Store, r wire.WorkspaceRequest) error { return s.SetSort(r.Name, r.Sort) }),
		write("POST /v1/workspaces/remove", func(s *Store, r wire.WorkspaceRequest) error { return s.Remove(r.Key) }),
	}
}
