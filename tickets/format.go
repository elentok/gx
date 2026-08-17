package tickets

import "fmt"

// FormatCost renders a dollar cost consistently across the codebase (Tickets
// UI, Queue UI, ralph-loop notifications).
func FormatCost(cost float64) string {
	return fmt.Sprintf("$%.2f", cost)
}
