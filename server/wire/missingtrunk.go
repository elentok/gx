package wire

const (
	// EventTrunkMissing streams once when a project with queued roots enters
	// Missing trunk. Address is the project; Message is the reason.
	EventTrunkMissing = "trunk-missing"
	// EventTrunkFound streams once when that project's trunk resolves again.
	EventTrunkFound = "trunk-found"
	// VerdictTrunkMissing is the explain verdict for a queued ticket of a
	// project in Missing trunk.
	VerdictTrunkMissing = "trunk missing"
	// VerdictMigrationBlocked is the explain verdict for a queued ticket of a
	// project whose migration failed.
	VerdictMigrationBlocked = "Migration blocked"
)
