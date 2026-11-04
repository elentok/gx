package wire

// EventMergeRolledBack streams when startup recovery found a merge that never
// reached the feature branch and undid what it left half done.
const EventMergeRolledBack = "land-rolled-back"
