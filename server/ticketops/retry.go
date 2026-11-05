package ticketops

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/elentok/gx/server/queue"
	"github.com/elentok/gx/server/wire"
)

// Retry serves the retry route. Retry on an answer-class pause whose agent is
// still running prompts that agent through the resume dispatch (status
// prompted); every other retry relaunches. An investigator's retry always
// relaunches.
func (o *Ops) Retry(web bool) http.HandlerFunc {
	relaunch := queue.HandlerAs(web, o.d.Writer.Relaunch)
	resume := o.ResumeHandler(web)
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var req struct {
			Address string `json:"address"`
		}
		if json.Unmarshal(body, &req) == nil && (web || r.Header.Get(wire.InvestigateHeader) == "") && o.PromptsOnRetry(req.Address) {
			resume(w, r)
			return
		}
		relaunch(w, r)
	}
}
