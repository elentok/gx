package wire

const (
	// EventDevChanged announces a change of the Dev supervisor's state or reason,
	// never a log line: pages refetch the snapshot on every event.
	EventDevChanged = "dev-changed"
	// EventReload tells every open page to reload: the UI served switched
	// between Vite and the built files.
	EventReload = "reload"
)
