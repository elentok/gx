package cmd

import (
	"encoding/json"
	"io"
)

func writeStamped(w io.Writer, v any, via, actor string) error {
	stamped, err := stampProvenance(v, via, actor)
	if err != nil {
		return err
	}
	return writeJSON(w, stamped)
}

func writeJSON(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}
