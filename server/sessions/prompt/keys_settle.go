package prompt

import (
	"cmp"
	"slices"
	"time"
)

// SettleScreen reads the screen every step until two consecutive reads match,
// or until cap. It returns the last rows read: a failing read or a screen that
// never settles still gives the last good read (none if no read succeeded).
func SettleScreen(read func() ([]string, error), step, cap time.Duration) []string {
	deadline := time.Now().Add(cap)
	var last []string
	have := false
	for {
		rows, err := read()
		if err != nil {
			return last
		}
		if have && slices.Equal(rows, last) {
			return rows
		}
		last, have = rows, true
		if !time.Now().Add(step).Before(deadline) {
			return last
		}
		time.Sleep(step)
	}
}

func (p *Prompt) settleRows(read func() ([]string, error)) []string {
	return SettleScreen(read,
		cmp.Or(p.cfg.KeysSettleStep, DefaultKeysSettleStep),
		cmp.Or(p.cfg.KeysSettleCap, DefaultKeysSettleCap))
}
