package wire

// StatsGrilling is the Grilling section: words and questions per day of the
// counted Design conversations. Days before the first counted item are
// no_data.
type StatsGrilling struct {
	Words     []StatsAmount `json:"words"`
	Questions []StatsAmount `json:"questions"`
}
