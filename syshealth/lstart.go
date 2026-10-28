package syshealth

import (
	"fmt"
	"strings"
	"time"
)

// ParseLstart turns `ps -o lstart` text (C locale, local time, for example
// "Wed Oct 28 14:21:31 2026") into Unix seconds.
func ParseLstart(s string) (int64, error) {
	// ps pads a single-digit day with a space.
	norm := strings.Join(strings.Fields(s), " ")
	t, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", norm, time.Local)
	if err != nil {
		return 0, fmt.Errorf("lstart %q: %w", s, err)
	}
	return t.Unix(), nil
}
