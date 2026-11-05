package cmdbase

import (
	"encoding/json"
	"io"
)

func WriteStamped(w io.Writer, v any, via, actor string) error {
	stamped, err := StampProvenance(v, via, actor)
	if err != nil {
		return err
	}
	return WriteJSON(w, stamped)
}

func WriteJSON(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}
