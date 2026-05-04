package commit

// CurrentRef returns the commit ref currently displayed by this model.
func (m Model) CurrentRef() string {
	return m.ref
}

