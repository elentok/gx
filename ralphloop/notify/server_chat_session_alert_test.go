package notify

import "testing"

func TestSessionAlertMessage_UrgencyFollowsTheFlagNotTheWord(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		needsAttention bool
		want           urgency
	}{
		{"stuck", "Blocked", true, urgencyAnswer},
		{"done", "Done", false, 0},
		{"flag wins over a Done word", "Done", true, urgencyAnswer},
		{"flag wins over a stuck word", "Blocked", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionAlertMessage("gx", "s", tt.status, "b", tt.needsAttention).urgency
			if got != tt.want {
				t.Errorf("urgency = %v, want %v", got, tt.want)
			}
		})
	}
}
