package limits

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/elentok/gx/server/wire"
)

func validLimitWindow(name string, w *wire.LimitWindow) error {
	if w == nil {
		return nil
	}
	if math.IsNaN(w.UsedPercentage) || math.IsInf(w.UsedPercentage, 0) || w.UsedPercentage < 0 || w.UsedPercentage > 100 {
		return fmt.Errorf("%s used_percentage %v is outside 0 to 100", name, w.UsedPercentage)
	}
	if w.ResetsAt <= 0 {
		return fmt.Errorf("%s resets_at %d is not positive", name, w.ResetsAt)
	}
	return nil
}

func (l *Limits) reportWrite(w http.ResponseWriter, r *http.Request) {
	var req wire.LimitsReport
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, c := range []struct {
		name string
		win  *wire.LimitWindow
	}{{"five_hour", req.FiveHour}, {"seven_day", req.SevenDay}} {
		if err := validLimitWindow(c.name, c.win); err != nil {
			wire.WriteJSON(w, wire.LimitsReportResult{Refused: true, Reason: wire.ReasonInvalidReport, Message: err.Error()})
			return
		}
	}
	now := time.Now().Unix()
	res, err := l.store.apply(req, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := l.history.recordClaude(req, res, now); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if limitsVisiblyChanged(res.FiveHour, now) || limitsVisiblyChanged(res.SevenDay, now) {
		l.throttle.Trigger()
	}
	stored := l.store.read()
	wire.WriteJSON(w, wire.LimitsReportResult{FiveHour: stored.FiveHour, SevenDay: stored.SevenDay})
}
