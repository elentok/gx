package tickets

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const legacyProvisionalDestinationKey = "provisional_destination"

// MigrateDestinationUnconfirmed renames the epic field provisional_destination
// to destination_unconfirmed in every epic ticket.md under root. The value is
// kept as it is. When both keys are present the new one wins and the old one is
// dropped. A rerun is a no-op.
func MigrateDestinationUnconfirmed(root string) ([]MigrationChange, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var changes []MigrationChange
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		epicPath := filepath.Join(root, entry.Name())
		if !LoadEpicTicketMD(epicPath).HasTicketMD {
			continue
		}
		renamed := false
		err := editEpicFrontmatter(epicPath, func(mapping *yaml.Node) (bool, error) {
			old := mappingKeyIndex(mapping, legacyProvisionalDestinationKey)
			if old < 0 {
				return false, nil
			}
			renamed = true
			if mappingKeyIndex(mapping, "destination_unconfirmed") >= 0 {
				mapping.Content = append(mapping.Content[:old], mapping.Content[old+2:]...)
			} else {
				mapping.Content[old].Value = "destination_unconfirmed"
			}
			return true, nil
		})
		if err != nil {
			return changes, err
		}
		if renamed {
			changes = append(changes, MigrationChange{
				Path:  filepath.Join(epicPath, "ticket.md"),
				Notes: []string{"provisional_destination renamed to destination_unconfirmed"},
			})
		}
	}
	return changes, nil
}
