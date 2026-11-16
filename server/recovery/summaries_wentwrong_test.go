package recovery

import "testing"

func TestGxWentWrong_Wording(t *testing.T) {
	tests := []struct {
		name string
		c    gxCounts
		want string
	}{
		{"none", gxCounts{}, "none recorded"},
		{"all one", gxCounts{resends: 1, relaunches: 1, launchFailures: 1, escalations: 1, finishBlocks: 1, refreshes: 1, retries: 1},
			"1 prompt resend, 1 relaunch, 1 launch failure, 1 recovery escalation, 1 finish block, 1 skill refresh, 1 skill retry"},
		{"all two", gxCounts{resends: 2, relaunches: 2, launchFailures: 2, escalations: 2, finishBlocks: 2, refreshes: 2, retries: 2},
			"2 prompt resends, 2 relaunches, 2 launch failures, 2 recovery escalations, 2 finish blocks, 2 skill refreshes, 2 skill retries"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gxWentWrong(tt.c); got != tt.want {
				t.Errorf("gxWentWrong = %q, want %q", got, tt.want)
			}
		})
	}
}
