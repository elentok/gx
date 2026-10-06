package tickets

import (
	"errors"
	"fmt"
	"strings"
)

// CheckBlockedBy is the one verdict on t's literal blocked_by refs, shared by
// `gx tickets validate` and the loader so both report the same message. A ref
// qualified with an epic ("epic/06") is malformed until cross-epic refs exist;
// a bare ref must name a ticket in e. Every bad ref is reported at once.
func (e Epic) CheckBlockedBy(t Ticket) error {
	index := e.byNumberAndSuffix()
	var errs []error
	for _, ref := range t.BlockedBy {
		if strings.Contains(ref, "/") {
			errs = append(errs, fmt.Errorf("ticket %s: blocked_by %q is malformed (cross-epic refs are not supported)", t.DisplayNumber(), ref))
			continue
		}
		num, letters := splitBlockedByToken(ref)
		if _, ok := index[siblingKey(num, letters)]; !ok {
			errs = append(errs, fmt.Errorf("ticket %s: blocked_by %q names no ticket in this epic", t.DisplayNumber(), ref))
		}
	}
	return errors.Join(errs...)
}
