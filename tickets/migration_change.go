package tickets

// MigrationChange reports what changed for one ticket file during Migrate,
// as a human-readable note per changed field.
type MigrationChange struct {
	Path  string
	Notes []string
}
