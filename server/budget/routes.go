package budget

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/elentok/gx/server/wire"
)

// Routes lists the budget status, override and increase endpoints. The getter
// is called per request, so a route can be listed before its Budget is built.
func Routes(get func() *Budget) []wire.Route {
	return []wire.Route{
		{Pattern: "GET /v1/budget", Handler: func(w http.ResponseWriter, r *http.Request) { get().statusRead(w, r) }},
		{Pattern: "POST /v1/budget/override", Handler: func(w http.ResponseWriter, r *http.Request) { get().overrideWrite(w, r) }},
		{Pattern: "POST /v1/budget/increase", Handler: func(w http.ResponseWriter, r *http.Request) { get().increaseWrite(w, r) }},
	}
}

func (b *Budget) statusRead(w http.ResponseWriter, _ *http.Request) {
	wire.WriteJSON(w, b.Status(time.Now()))
}

func (b *Budget) overrideWrite(w http.ResponseWriter, _ *http.Request) {
	res, err := b.override(time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}

func (b *Budget) increaseWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.BudgetIncreaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := b.increase(time.Now(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wire.WriteJSON(w, res)
}
