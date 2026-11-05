package ralphloop

import "time"

// pausedPollInterval is how often a paused run (nothing runnable, something
// human-clearable) re-reads the epic looking for a status a person has
// cleared. Slow on purpose: every pass writes a scheduler-scan line to the run
// log, and a pause can last hours.
const pausedPollInterval = 30 * time.Second

// QueuePauseLabel identifies the in-process pause controlled by the Queue UI.
const QueuePauseLabel = "queue"

// BudgetPauseLabel identifies the in-process pause controlled by the
// soft-limit budget check — a separate label from QueuePauseLabel so a
// budget pause and a manual pause never clear each other via ForceResume.
const BudgetPauseLabel = "budget"

// BudgetHardPauseLabel identifies the in-process pause controlled by the
// hard-limit budget kill — a separate label from both QueuePauseLabel and
// BudgetPauseLabel so a hard-limit pause, a soft-limit pause, and a manual
// pause never clear each other via ForceResume.
const BudgetHardPauseLabel = "budget-hard"
