package wire

// QueueMode is the queue's mode, as reported in QueueResult.Mode.
type QueueMode string

const (
	QueueModeRunning QueueMode = "running"
	QueueModePaused  QueueMode = "paused"
)
