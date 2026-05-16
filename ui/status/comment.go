package status

func (m *Model) commentLocationAndBody() (string, []string, string, bool) {
	diffModel := m.diffarea.ActiveSectionModel()
	loc, body, yankErr := diffModel.FocusedLocationAndBody()
	if yankErr == "" {
		return loc, body, "", true
	}
	if len(diffModel.DataRef().RawLines) > 0 {
		return "", diffModel.DataRef().RawLines, "", true
	}
	return "", nil, string(yankErr), false
}
